// Package cmd defines the envault command line.
//
// Commands are thin: each one parses flags, calls into internal/…, and prints.
// Anything a command does that the browser UI also does belongs in the shared
// package, not here, so the two can never drift apart.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/setup"
	"github.com/akhshyganesh/envault/internal/tui"
)

// Version and BuildDate are injected at build time via -ldflags. A plain
// `go build` leaves them at these defaults; use `make build` when the version
// string matters.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:          "envault",
	Short:        "envault — your .env files, safely vaulted",
	SilenceUsage: true,
	// Bare `envault` is the front door: the wizard if there is no vault yet,
	// the browser if there is.
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(config.VaultDir()); os.IsNotExist(err) {
			return setup.Run()
		}
		return tui.Run()
	},
}

// Execute runs the command line and is the process's only exit point.
func Execute() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("envault {{.Version}}\n")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
