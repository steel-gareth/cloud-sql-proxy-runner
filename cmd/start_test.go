package cmd

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"cloud-sql-proxy-runner/internal/config"
	"cloud-sql-proxy-runner/internal/proxy"
)

var (
	proxyA = config.ProxyEntry{Instance: "proj:us-central1:db-a", Port: 5432, Secret: "secret-a"}
	proxyB = config.ProxyEntry{Instance: "proj:us-central1:db-b", Port: 5433, Secret: "secret-b"}
	proxyC = config.ProxyEntry{Instance: "proj:us-central1:db-c", Port: 5434, Secret: "secret-c"}
)

// deadPID returns a PID that is not running by spawning and waiting for a short-lived process.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawning dead process: %v", err)
	}
	return cmd.ProcessState.Pid()
}

// writeState is a test helper that writes both PID and state files.
func writeState(t *testing.T, dir string, pid int, proxies []config.ProxyEntry) {
	t.Helper()
	if err := proxy.WritePID(dir, pid); err != nil {
		t.Fatalf("writing PID: %v", err)
	}
	if err := proxy.WriteState(dir, &proxy.DaemonState{
		PID:       pid,
		StartedAt: time.Now().UTC(),
		Proxies:   proxies,
	}); err != nil {
		t.Fatalf("writing state: %v", err)
	}
}

// --- checkDaemon tests ---

func TestCheckDaemon_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA})
	if action != daemonStart {
		t.Errorf("expected daemonStart, got %d", action)
	}
	if pid != 0 {
		t.Errorf("expected pid 0, got %d", pid)
	}
}

func TestCheckDaemon_StalePID(t *testing.T) {
	dir := t.TempDir()
	dead := deadPID(t)
	writeState(t, dir, dead, []config.ProxyEntry{proxyA})

	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA})
	if action != daemonStart {
		t.Errorf("expected daemonStart for dead process, got %d", action)
	}
	if pid != 0 {
		t.Errorf("expected pid 0, got %d", pid)
	}
}

func TestCheckDaemon_RunningWithSameConfig(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid() // test process is alive
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA, proxyB})

	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA, proxyB})
	if action != daemonKeep {
		t.Errorf("expected daemonKeep, got %d", action)
	}
	if pid != livePID {
		t.Errorf("expected pid %d, got %d", livePID, pid)
	}
}

func TestCheckDaemon_RunningWithReorderedConfig(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA, proxyB, proxyC})

	// Same proxies in different order should be kept
	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyC, proxyA, proxyB})
	if action != daemonKeep {
		t.Errorf("expected daemonKeep for reordered config, got %d", action)
	}
	if pid != livePID {
		t.Errorf("expected pid %d, got %d", livePID, pid)
	}
}

func TestCheckDaemon_RunningWithProxyAdded(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA})

	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA, proxyB})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when proxy added, got %d", action)
	}
	if pid != livePID {
		t.Errorf("expected pid %d, got %d", livePID, pid)
	}
}

func TestCheckDaemon_RunningWithProxyRemoved(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA, proxyB})

	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when proxy removed, got %d", action)
	}
	if pid != livePID {
		t.Errorf("expected pid %d, got %d", livePID, pid)
	}
}

func TestCheckDaemon_RunningWithPortChanged(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA})

	changed := config.ProxyEntry{Instance: proxyA.Instance, Port: 9999, Secret: proxyA.Secret}
	action, _ := checkDaemon(dir, []config.ProxyEntry{changed})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when port changed, got %d", action)
	}
}

func TestCheckDaemon_RunningWithSecretChanged(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA})

	changed := config.ProxyEntry{Instance: proxyA.Instance, Port: proxyA.Port, Secret: "new-secret"}
	action, _ := checkDaemon(dir, []config.ProxyEntry{changed})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when secret changed, got %d", action)
	}
}

func TestCheckDaemon_RunningWithProxyReplaced(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA, proxyB})

	// Replace B with C
	action, _ := checkDaemon(dir, []config.ProxyEntry{proxyA, proxyC})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when proxy replaced, got %d", action)
	}
}

func TestCheckDaemon_RunningWithMissingStateFile(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	// Write only PID file, no state.json
	if err := proxy.WritePID(dir, livePID); err != nil {
		t.Fatalf("writing PID: %v", err)
	}

	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when state.json missing, got %d", action)
	}
	if pid != livePID {
		t.Errorf("expected pid %d, got %d", livePID, pid)
	}
}

func TestCheckDaemon_RunningWithCorruptStateFile(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	if err := proxy.WritePID(dir, livePID); err != nil {
		t.Fatalf("writing PID: %v", err)
	}
	// Write garbage to state.json
	if err := os.WriteFile(dir+"/state.json", []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("writing corrupt state: %v", err)
	}

	action, pid := checkDaemon(dir, []config.ProxyEntry{proxyA})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart when state.json corrupt, got %d", action)
	}
	if pid != livePID {
		t.Errorf("expected pid %d, got %d", livePID, pid)
	}
}

func TestCheckDaemon_RunningWithCompletelyDifferentConfig(t *testing.T) {
	dir := t.TempDir()
	livePID := os.Getpid()
	writeState(t, dir, livePID, []config.ProxyEntry{proxyA})

	// Completely different proxy
	action, _ := checkDaemon(dir, []config.ProxyEntry{proxyB})
	if action != daemonRestart {
		t.Errorf("expected daemonRestart for completely different config, got %d", action)
	}
}

// --- stopDaemon tests ---

func TestStopDaemon_TerminatesProcess(t *testing.T) {
	dir := t.TempDir()

	// Spawn a long-running process to stop
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleep process: %v", err)
	}
	pid := cmd.Process.Pid
	// Reap the child in the background so it doesn't become a zombie after kill.
	go cmd.Wait()
	writeState(t, dir, pid, []config.ProxyEntry{proxyA})

	// Verify it's alive
	if !proxy.IsRunning(pid) {
		t.Fatal("sleep process should be running before stop")
	}

	if err := stopDaemon(pid, dir); err != nil {
		t.Fatalf("stopDaemon: %v", err)
	}

	// Give the reaper goroutine a moment to collect the exit status
	time.Sleep(50 * time.Millisecond)

	// Process should be dead
	if proxy.IsRunning(pid) {
		t.Error("process should not be running after stopDaemon")
	}

	// State files should be cleaned up
	if _, err := proxy.ReadPID(dir); err == nil {
		t.Error("PID file should be removed after stopDaemon")
	}
	if _, err := proxy.ReadState(dir); err == nil {
		t.Error("state file should be removed after stopDaemon")
	}
}

func TestStopDaemon_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	dead := deadPID(t)
	writeState(t, dir, dead, []config.ProxyEntry{proxyA})

	// Should not error on already-dead process
	if err := stopDaemon(dead, dir); err != nil {
		t.Fatalf("stopDaemon on dead process: %v", err)
	}

	// State files should be cleaned up
	if _, err := proxy.ReadPID(dir); err == nil {
		t.Error("PID file should be removed")
	}
}

// --- proxiesEqual tests ---

func TestProxiesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []config.ProxyEntry
		want bool
	}{
		{
			name: "both empty",
			a:    []config.ProxyEntry{},
			b:    []config.ProxyEntry{},
			want: true,
		},
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "nil vs empty",
			a:    nil,
			b:    []config.ProxyEntry{},
			want: true,
		},
		{
			name: "identical single entry",
			a:    []config.ProxyEntry{proxyA},
			b:    []config.ProxyEntry{proxyA},
			want: true,
		},
		{
			name: "identical multiple entries",
			a:    []config.ProxyEntry{proxyA, proxyB, proxyC},
			b:    []config.ProxyEntry{proxyA, proxyB, proxyC},
			want: true,
		},
		{
			name: "same entries different order",
			a:    []config.ProxyEntry{proxyA, proxyB, proxyC},
			b:    []config.ProxyEntry{proxyC, proxyA, proxyB},
			want: true,
		},
		{
			name: "same entries reversed",
			a:    []config.ProxyEntry{proxyA, proxyB},
			b:    []config.ProxyEntry{proxyB, proxyA},
			want: true,
		},
		{
			name: "proxy added",
			a:    []config.ProxyEntry{proxyA},
			b:    []config.ProxyEntry{proxyA, proxyB},
			want: false,
		},
		{
			name: "proxy removed",
			a:    []config.ProxyEntry{proxyA, proxyB},
			b:    []config.ProxyEntry{proxyA},
			want: false,
		},
		{
			name: "proxy replaced",
			a:    []config.ProxyEntry{proxyA, proxyB},
			b:    []config.ProxyEntry{proxyA, proxyC},
			want: false,
		},
		{
			name: "port changed",
			a:    []config.ProxyEntry{proxyA},
			b:    []config.ProxyEntry{{Instance: proxyA.Instance, Port: 9999, Secret: proxyA.Secret}},
			want: false,
		},
		{
			name: "secret changed",
			a:    []config.ProxyEntry{proxyA},
			b:    []config.ProxyEntry{{Instance: proxyA.Instance, Port: proxyA.Port, Secret: "new-secret"}},
			want: false,
		},
		{
			name: "instance changed",
			a:    []config.ProxyEntry{proxyA},
			b:    []config.ProxyEntry{{Instance: "other:us-central1:db", Port: proxyA.Port, Secret: proxyA.Secret}},
			want: false,
		},
		{
			name: "empty vs non-empty",
			a:    []config.ProxyEntry{},
			b:    []config.ProxyEntry{proxyA},
			want: false,
		},
		{
			name: "all fields differ",
			a:    []config.ProxyEntry{proxyA},
			b:    []config.ProxyEntry{proxyB},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proxiesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("proxiesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
