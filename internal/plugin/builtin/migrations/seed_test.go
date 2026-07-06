package migrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/plugin/builtin/migrations"
)

// fakeExecutor records every (databaseURL, sql) call it receives, and
// returns failFor's error (if any) when its sql content matches — good
// enough to test Seed's ordering and error propagation without a live
// postgres connection.
type fakeExecutorCall struct {
	databaseURL string
	sql         string
}

func newFakeExecutor(failOn string, failErr error) (*[]fakeExecutorCall, migrations.SeedExecutor) {
	calls := &[]fakeExecutorCall{}
	exec := func(databaseURL string, sqlBytes []byte) error {
		*calls = append(*calls, fakeExecutorCall{databaseURL: databaseURL, sql: string(sqlBytes)})
		if failOn != "" && string(sqlBytes) == failOn {
			return failErr
		}
		return nil
	}
	return calls, exec
}

func writeSeedFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSeedWithExecutor_NoSeedsDir_ReturnsPointedError(t *testing.T) {
	root := t.TempDir()
	_, exec := newFakeExecutor("", nil)

	err := migrations.SeedWithExecutor(root, "postgres://x", exec)
	if err == nil {
		t.Fatal("expected an error when seeds/ does not exist")
	}
	if err != migrations.ErrNoSeedsDir {
		t.Errorf("expected ErrNoSeedsDir, got: %v", err)
	}
}

func TestSeedWithExecutor_RunsFilesInLexicalOrder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "seeds")
	writeSeedFile(t, dir, "0002_users.sql", "insert users")
	writeSeedFile(t, dir, "0001_orgs.sql", "insert orgs")
	writeSeedFile(t, dir, "readme.txt", "not sql, ignored")

	calls, exec := newFakeExecutor("", nil)
	if err := migrations.SeedWithExecutor(root, "postgres://x", exec); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("expected 2 seed files run, got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].sql != "insert orgs" || (*calls)[1].sql != "insert users" {
		t.Errorf("expected lexical order (orgs before users), got: %+v", *calls)
	}
	for _, c := range *calls {
		if c.databaseURL != "postgres://x" {
			t.Errorf("expected databaseURL threaded through, got %q", c.databaseURL)
		}
	}
}

func TestSeedWithExecutor_EmptySeedsDir_ReturnsPointedError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "seeds"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, exec := newFakeExecutor("", nil)

	err := migrations.SeedWithExecutor(root, "postgres://x", exec)
	if err == nil {
		t.Fatal("expected an error for an empty seeds directory")
	}
}

func TestSeedWithExecutor_ExecutorFailure_StopsAndNamesFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "seeds")
	writeSeedFile(t, dir, "0001_orgs.sql", "insert orgs")
	writeSeedFile(t, dir, "0002_users.sql", "insert users")

	wantErr := fmt.Errorf("boom")
	_, exec := newFakeExecutor("insert orgs", wantErr)

	err := migrations.SeedWithExecutor(root, "postgres://x", exec)
	if err == nil {
		t.Fatal("expected an error when a seed file fails")
	}
	if !strings.Contains(err.Error(), "0001_orgs.sql") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected the failing file named in the error, got: %v", err)
	}
}

func TestSeedFileWithExecutor_RunsExactlyOneFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "seeds")
	writeSeedFile(t, dir, "0001_orgs.sql", "insert orgs")
	writeSeedFile(t, dir, "0002_users.sql", "insert users")

	calls, exec := newFakeExecutor("", nil)
	if err := migrations.SeedFileWithExecutor(root, "postgres://x", "seeds/0002_users.sql", exec); err != nil {
		t.Fatalf("SeedFile: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0].sql != "insert users" {
		t.Fatalf("expected exactly the named file run, got: %+v", *calls)
	}
}
