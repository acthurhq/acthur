package migrations

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

// ErrNoSeedsDir is returned by Seed (and SeedWithExecutor) when the
// project's seeds/ directory does not exist.
var ErrNoSeedsDir = errors.New(
	"no seeds/ directory found — create one with seeds/*.sql files to use `acthur db seed`")

// seedsDir returns the project-root seeds/ directory for root.
func seedsDir(root string) string {
	return filepath.Join(root, "seeds")
}

// SeedExecutor runs one seed file's raw SQL content against databaseURL.
// It is injected so Seed/SeedFile are unit-testable without a live postgres
// connection (the seam pattern this package already uses for docker/db
// commands — see deploy.Runner); production uses ExecSQLFile.
type SeedExecutor func(databaseURL string, sqlBytes []byte) error

// ExecSQLFile opens a real postgres connection and executes contents as a
// single batch. It is the production SeedExecutor.
func ExecSQLFile(databaseURL string, contents []byte) error {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("opening database connection: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(string(contents)); err != nil {
		return fmt.Errorf("executing seed sql: %w", err)
	}
	return nil
}

// listSeedFiles returns every seeds/*.sql filename (not full path) in
// root's seeds/ directory, lexically sorted so seed order is deterministic
// and reproducible across runs.
func listSeedFiles(root string) ([]string, error) {
	dir := seedsDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSeedsDir
		}
		return nil, fmt.Errorf("reading seeds directory: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// SeedWithExecutor runs every seeds/*.sql file in root's seeds/ directory,
// in lexical order, via exec. Seed and the `acthur db seed` command both
// build on this.
func SeedWithExecutor(root, databaseURL string, exec SeedExecutor) error {
	files, err := listSeedFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("seeds directory %s has no .sql files", seedsDir(root))
	}
	for _, name := range files {
		path := filepath.Join(seedsDir(root), name)
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading seed file %s: %w", path, err)
		}
		if err := exec(databaseURL, contents); err != nil {
			return fmt.Errorf("running seed file %s: %w", name, err)
		}
	}
	return nil
}

// Seed runs every seeds/*.sql file in root's seeds/ directory, in lexical
// order, against a live postgres database.
func Seed(root, databaseURL string) error {
	return SeedWithExecutor(root, databaseURL, ExecSQLFile)
}

// SeedFileWithExecutor runs exactly one seed file (an absolute path, or one
// relative to root) via exec.
func SeedFileWithExecutor(root, databaseURL, path string, exec SeedExecutor) error {
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, full)
	}
	contents, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("reading seed file %s: %w", full, err)
	}
	if err := exec(databaseURL, contents); err != nil {
		return fmt.Errorf("running seed file %s: %w", full, err)
	}
	return nil
}

// SeedFile runs exactly one seed file against a live postgres database.
func SeedFile(root, databaseURL, path string) error {
	return SeedFileWithExecutor(root, databaseURL, path, ExecSQLFile)
}
