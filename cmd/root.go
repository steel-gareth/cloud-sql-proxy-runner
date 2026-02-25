package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "cloud-sql-proxy-runner",
	Short: "Manage Cloud SQL proxy connections",
	Long:  "Start, stop, and list Cloud SQL proxy connections defined in a YAML config.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	built := buildTime
	if t, err := time.Parse("20060102150405", buildTime); err == nil {
		built = t.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, gitCommit, built)

	home, _ := os.UserHomeDir()
	defaultConfig := filepath.Join(home, ".config", "cloud-sql-proxy-runner", "config.yaml")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfig, "path to config file")
}
