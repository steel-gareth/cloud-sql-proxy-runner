package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud-proxy/internal/config"
)

func TestWriteReadPIDRoundtrip(t *testing.T) {
	dir := t.TempDir()
	pid := 12345
	if err := WritePID(dir, pid); err != nil {
		t.Fatalf("WritePID: %v", err)
	}
	got, err := ReadPID(dir)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if got != pid {
		t.Errorf("expected pid %d, got %d", pid, got)
	}
}

func TestWriteReadStateRoundtrip(t *testing.T) {
	dir := t.TempDir()
	state := &DaemonState{
		PID:       42,
		StartedAt: time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC),
		Proxies: []config.ProxyEntry{
			{Instance: "proj:region:db", Port: 5432, Secret: "pw"},
		},
	}
	if err := WriteState(dir, state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.PID != state.PID {
		t.Errorf("expected pid %d, got %d", state.PID, got.PID)
	}
	if !got.StartedAt.Equal(state.StartedAt) {
		t.Errorf("expected started_at %v, got %v", state.StartedAt, got.StartedAt)
	}
	if len(got.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(got.Proxies))
	}
	if got.Proxies[0].Instance != "proj:region:db" {
		t.Errorf("expected instance 'proj:region:db', got %q", got.Proxies[0].Instance)
	}
}

func TestIsRunning_OwnPID(t *testing.T) {
	if !IsRunning(os.Getpid()) {
		t.Error("expected IsRunning to return true for own PID")
	}
}

func TestIsRunning_NonExistentPID(t *testing.T) {
	// PID 99999999 is extremely unlikely to exist
	if IsRunning(99999999) {
		t.Error("expected IsRunning to return false for non-existent PID")
	}
}

func TestCleanupStale(t *testing.T) {
	dir := t.TempDir()
	// Write a PID file with a non-existent PID
	if err := WritePID(dir, 99999999); err != nil {
		t.Fatalf("WritePID: %v", err)
	}
	state := &DaemonState{PID: 99999999, StartedAt: time.Now()}
	if err := WriteState(dir, state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	if err := CleanupStale(dir); err != nil {
		t.Fatalf("CleanupStale: %v", err)
	}

	// PID file should be gone
	if _, err := os.Stat(filepath.Join(dir, PIDFile)); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed")
	}
	// State file should be gone
	if _, err := os.Stat(filepath.Join(dir, StateFile)); !os.IsNotExist(err) {
		t.Error("expected state file to be removed")
	}
}

func TestStateDirCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "state")
	if err := EnsureStateDir(dir); err != nil {
		t.Fatalf("EnsureStateDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
