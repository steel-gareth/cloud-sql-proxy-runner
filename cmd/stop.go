package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"cloud-proxy/internal/proxy"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the proxy daemon",
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	stateDir := proxy.StateDir()

	pid, err := proxy.ReadPID(stateDir)
	if err != nil || !proxy.IsRunning(pid) {
		// Clean up stale files if any
		if err == nil {
			proxy.RemoveStateFiles(stateDir)
		}
		fmt.Println("No daemon is running.")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("No daemon is running.")
		return nil
	}

	// Send SIGTERM
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		proxy.RemoveStateFiles(stateDir)
		fmt.Println("No daemon is running.")
		return nil
	}

	// Wait up to 5 seconds for exit
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !proxy.IsRunning(pid) {
			proxy.RemoveStateFiles(stateDir)
			fmt.Println("Daemon stopped.")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill
	proc.Signal(syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)
	proxy.RemoveStateFiles(stateDir)
	fmt.Println("Daemon stopped.")
	return nil
}
