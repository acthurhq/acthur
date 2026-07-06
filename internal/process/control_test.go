package process_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acthur/acthur/internal/process"
)

func TestFileLogSink_AppendsLinesToPerNodeFile(t *testing.T) {
	root := t.TempDir()
	sink, err := process.FileLogSink(root)
	if err != nil {
		t.Fatalf("FileLogSink: %v", err)
	}

	sink("api", "first line")
	sink("api", "second line")
	sink("worker", "worker line")

	apiData, err := os.ReadFile(process.LogPath(root, "api"))
	if err != nil {
		t.Fatalf("read api log: %v", err)
	}
	if got := string(apiData); got != "first line\nsecond line\n" {
		t.Fatalf("unexpected api log content: %q", got)
	}

	workerData, err := os.ReadFile(process.LogPath(root, "worker"))
	if err != nil {
		t.Fatalf("read worker log: %v", err)
	}
	if got := string(workerData); got != "worker line\n" {
		t.Fatalf("unexpected worker log content: %q", got)
	}
}

func TestLogPath_IsUnderActhurLogsDir(t *testing.T) {
	got := process.LogPath("/proj", "api")
	want := filepath.Join("/proj", ".acthur", "logs", "api.log")
	if got != want {
		t.Fatalf("LogPath = %q, want %q", got, want)
	}
}

func TestWriteReadRemovePIDFile_RoundTrips(t *testing.T) {
	root := t.TempDir()

	if _, err := process.ReadPIDFile(root, "api"); err == nil {
		t.Fatal("expected error reading a pidfile that doesn't exist yet")
	}

	if err := process.WritePIDFile(root, "api", 4242); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	pid, err := process.ReadPIDFile(root, "api")
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != 4242 {
		t.Fatalf("expected pid 4242, got %d", pid)
	}

	if err := process.RemovePIDFile(root, "api"); err != nil {
		t.Fatalf("RemovePIDFile: %v", err)
	}
	if _, err := process.ReadPIDFile(root, "api"); err == nil {
		t.Fatal("expected error reading a pidfile after it was removed")
	}
}

func TestRemovePIDFile_NoErrorWhenAlreadyAbsent(t *testing.T) {
	root := t.TempDir()
	if err := process.RemovePIDFile(root, "never-existed"); err != nil {
		t.Fatalf("expected no error removing a nonexistent pidfile, got %v", err)
	}
}

func TestReadPIDFile_ErrorsOnCorruptContent(t *testing.T) {
	root := t.TempDir()
	path := process.PIDPath(root, "api")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatalf("write corrupt pidfile: %v", err)
	}

	if _, err := process.ReadPIDFile(root, "api"); err == nil {
		t.Fatal("expected error reading a corrupt pidfile")
	}
}
