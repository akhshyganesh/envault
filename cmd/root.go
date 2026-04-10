// Package cmd contains all envault CLI commands.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/setup"
	"github.com/akhshyganesh/envault/internal/tui"
)

// Version and BuildDate are injected at build time via ldflags.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:          "envault",
	Short:        "🔒 envault — your .env files, safely vaulted",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(config.VaultDir()); os.IsNotExist(err) {
			// First run: vault doesn't exist — launch the setup wizard.
			return setup.Run()
		}
		// Already set up: open the interactive TUI.
		return tui.Run()
	},
}

// Execute is the application entry point.
func Execute() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("envault {{.Version}}\n")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
