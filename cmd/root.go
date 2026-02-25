package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "cloud-proxy",
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
	home, _ := os.UserHomeDir()
	defaultConfig := filepath.Join(home, ".config", "cloud-proxy", "config.yaml")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfig, "path to config file")
}
