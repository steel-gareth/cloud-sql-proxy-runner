package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud-sql-proxy-runner/internal/config"
)

const (
	DefaultStateDir = ".cloud-sql-proxy-runner"
	PIDFile         = "daemon.pid"
	StateFile       = "state.json"
	LogFile         = "daemon.log"
)

type DaemonState struct {
	PID       int                  `json:"pid"`
	StartedAt time.Time            `json:"started_at"`
	Proxies   []config.ProxyEntry  `json:"proxies"`
}

func StateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultStateDir
	}
	return filepath.Join(home, DefaultStateDir)
}

func EnsureStateDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

func WritePID(dir string, pid int) error {
	if err := EnsureStateDir(dir); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, PIDFile), []byte(strconv.Itoa(pid)), 0644)
}

func ReadPID(dir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(dir, PIDFile))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func WriteState(dir string, state *DaemonState) error {
	if err := EnsureStateDir(dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, StateFile), data, 0644)
}

func ReadState(dir string) (*DaemonState, error) {
	data, err := os.ReadFile(filepath.Join(dir, StateFile))
	if err != nil {
		return nil, err
	}
	var state DaemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func IsRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func CleanupStale(dir string) error {
	pid, err := ReadPID(dir)
	if err != nil {
		return nil // no PID file, nothing to clean
	}
	if IsRunning(pid) {
		return fmt.Errorf("daemon is still running (pid %d)", pid)
	}
	// Stale PID file - clean up
	os.Remove(filepath.Join(dir, PIDFile))
	os.Remove(filepath.Join(dir, StateFile))
	return nil
}

func RemoveStateFiles(dir string) {
	os.Remove(filepath.Join(dir, PIDFile))
	os.Remove(filepath.Join(dir, StateFile))
}

func LogPath(dir string) string {
	return filepath.Join(dir, LogFile)
}
