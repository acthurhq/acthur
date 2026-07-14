package migrations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// migrationsDir returns the project-root migrations/ directory for root.
func migrationsDir(root string) string {
	return filepath.Join(root, "migrations")
}

// newMigrate constructs a *migrate.Migrate wired to the project's
// migrations/ directory (as a file:// source) and databaseURL (as the
// postgres target). Callers must Close() the returned instance.
func newMigrate(root, databaseURL string) (*migrate.Migrate, error) {
	dir := migrationsDir(root)
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("migrations directory %s does not exist — run 'acthur add migrations' first: %w", dir, err)
	}
	sourceURL := "file://" + filepath.ToSlash(dir)
	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("constructing migrate instance: %w", err)
	}
	return m, nil
}

// Migrate applies all pending "up" migrations.
func Migrate(root, databaseURL string) error {
	m, err := newMigrate(root, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

// Rollback reverts exactly one applied migration.
func Rollback(root, databaseURL string) error {
	m, err := newMigrate(root, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rolling back migration: %w", err)
	}
	return nil
}

// DownAll reverts every applied migration — the first step of `acthur db
// reset` (down-all, then up-all, then seed).
func DownAll(root, databaseURL string) error {
	m, err := newMigrate(root, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rolling back all migrations: %w", err)
	}
	return nil
}

// Status reports the currently-applied migration version and whether the
// last migration left the database in a dirty (partially-applied) state.
func Status(root, databaseURL string) (version uint, dirty bool, err error) {
	m, mErr := newMigrate(root, databaseURL)
	if mErr != nil {
		return 0, false, mErr
	}
	defer func() { _, _ = m.Close() }()
	version, dirty, err = m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reading migration status: %w", err)
	}
	return version, dirty, nil
}

// ---------------------------------------------------------------------------
// db create <name> — numbered empty up/down pair
// ---------------------------------------------------------------------------

var migrationFileRe = regexp.MustCompile(`^(\d{4,})_`)
var upMigrationFileRe = regexp.MustCompile(`^(\d{4,})_.+\.up\.sql$`)

// LatestVersion returns the highest version represented by an up migration
// under root/migrations. present is false when the directory is absent or
// contains no up migrations, allowing callers to avoid touching a database
// for projects that do not use migrations.
func LatestVersion(root string) (version uint, present bool, err error) {
	entries, err := os.ReadDir(migrationsDir(root))
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reading migrations directory: %w", err)
	}

	var latest uint64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := upMigrationFileRe.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		candidate, parseErr := strconv.ParseUint(match[1], 10, 64)
		if parseErr != nil {
			return 0, false, fmt.Errorf("reading migration version from %q: %w", entry.Name(), parseErr)
		}
		if candidate > uint64(^uint(0)) {
			return 0, false, fmt.Errorf("migration version in %q exceeds platform uint range", entry.Name())
		}
		if candidate > latest {
			latest = candidate
		}
	}
	if latest == 0 {
		return 0, false, nil
	}
	return uint(latest), true, nil
}

// CreateNext writes the next-numbered empty up/down migration pair named
// name into the project's migrations/ directory, creating the directory if
// needed. It returns the two file paths written.
func CreateNext(root, name string) (upPath, downPath string, err error) {
	dir := migrationsDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating migrations directory: %w", err)
	}

	next, err := nextMigrationNumber(dir)
	if err != nil {
		return "", "", err
	}

	slug := slugify(name)
	upPath = filepath.Join(dir, fmt.Sprintf("%04d_%s.up.sql", next, slug))
	downPath = filepath.Join(dir, fmt.Sprintf("%04d_%s.down.sql", next, slug))

	if err := os.WriteFile(upPath, []byte(fmt.Sprintf("-- %04d_%s: up\n", next, slug)), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", upPath, err)
	}
	if err := os.WriteFile(downPath, []byte(fmt.Sprintf("-- %04d_%s: down\n", next, slug)), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", downPath, err)
	}
	return upPath, downPath, nil
}

// nextMigrationNumber scans dir for existing NNNN_*.sql files and returns
// one past the highest number found, or 1 if the directory has none yet.
func nextMigrationNumber(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading migrations directory: %w", err)
	}

	max := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}

// slugify lower-cases name and replaces anything that is not alphanumeric
// with an underscore, matching the migrations/NNNN_<desc>.up.sql convention.
func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	s := b.String()
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return strings.Trim(s, "_")
}
