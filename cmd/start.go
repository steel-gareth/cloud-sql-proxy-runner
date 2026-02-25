package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud-proxy/internal/config"
	"cloud-proxy/internal/preflight"
	"cloud-proxy/internal/proxy"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/spf13/cobra"
)

var daemonFlag bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the proxy daemon",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "internal: run as daemon process")
	startCmd.Flags().MarkHidden("daemon")
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	if daemonFlag {
		return runDaemon()
	}
	return runStartForeground()
}

func runStartForeground() error {
	ctx := context.Background()

	// Preflight: check ADC
	if err := preflight.CheckADC(ctx, preflight.DefaultCredentialFinder); err != nil {
		return err
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	stateDir := proxy.StateDir()

	// Check for existing daemon
	pid, err := proxy.ReadPID(stateDir)
	if err == nil && proxy.IsRunning(pid) {
		// Compare running config with desired config
		state, stateErr := proxy.ReadState(stateDir)
		if stateErr == nil && proxiesEqual(state.Proxies, cfg.Proxies) {
			fmt.Printf("Daemon already running (pid %d)\n", pid)
			return nil
		}
		// Config has changed (or state unreadable) — restart daemon
		fmt.Println("Config changed, restarting daemon...")
		if err := stopDaemon(pid, stateDir); err != nil {
			return fmt.Errorf("stopping old daemon: %w", err)
		}
	}

	// Clean up stale PID file if any
	proxy.CleanupStale(stateDir)

	// Daemonize: re-exec with --daemon flag
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	if err := proxy.EnsureStateDir(stateDir); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	logFile, err := os.OpenFile(proxy.LogPath(stateDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	daemonCmd := exec.Command(execPath, "start", "--daemon", "--config", configPath)
	daemonCmd.Stdout = logFile
	daemonCmd.Stderr = logFile
	daemonCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := daemonCmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting daemon: %w", err)
	}
	logFile.Close()

	// Wait briefly for daemon to start and confirm ports
	time.Sleep(500 * time.Millisecond)

	for _, p := range cfg.Proxies {
		name := instanceShortName(p.Instance)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", p.Port), 2*time.Second)
		if err != nil {
			fmt.Printf("%-8s failed to start on port %d\n", name+":", p.Port)
			continue
		}
		conn.Close()
		fmt.Printf("%-8s started on port %d\n", name+":", p.Port)
	}

	return nil
}

func instanceShortName(instance string) string {
	parts := strings.Split(instance, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	return instance
}

func runDaemon() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stateDir := proxy.StateDir()

	// Write PID
	if err := proxy.WritePID(stateDir, os.Getpid()); err != nil {
		return fmt.Errorf("writing PID: %w", err)
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// Create Cloud SQL dialer
	dialer, err := cloudsqlconn.NewDialer(ctx)
	if err != nil {
		return fmt.Errorf("creating Cloud SQL dialer: %w", err)
	}
	defer dialer.Close()

	// Wrap the real dialer to match our interface
	d := &realDialer{dialer: dialer}

	// Start listeners
	var listeners []*proxy.Listener
	for _, p := range cfg.Proxies {
		l := proxy.NewListener(p.Instance, p.Port, d)
		if err := l.Start(ctx); err != nil {
			log.Printf("failed to start listener for %s on port %d: %v", p.Instance, p.Port, err)
			// Clean up already-started listeners
			for _, started := range listeners {
				started.Close()
			}
			proxy.RemoveStateFiles(stateDir)
			return err
		}
		listeners = append(listeners, l)
		log.Printf("listening on port %d for %s", p.Port, p.Instance)
	}

	// Write state file
	state := &proxy.DaemonState{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
		Proxies:   cfg.Proxies,
	}
	if err := proxy.WriteState(stateDir, state); err != nil {
		log.Printf("warning: failed to write state file: %v", err)
	}

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	log.Println("shutting down...")
	cancel()
	for _, l := range listeners {
		l.Close()
	}
	proxy.RemoveStateFiles(stateDir)
	log.Println("daemon stopped")
	return nil
}

type realDialer struct {
	dialer *cloudsqlconn.Dialer
}

func (r *realDialer) Dial(ctx context.Context, instance string) (net.Conn, error) {
	return r.dialer.Dial(ctx, instance)
}

func (r *realDialer) Close() error {
	return r.dialer.Close()
}

func proxiesEqual(a, b []config.ProxyEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
