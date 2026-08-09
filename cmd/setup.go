package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/setup"
	"github.com/akhshyganesh/envault/internal/tui"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Interactive setup — watch directories, interval, startup service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return setup.Run()
	},
}

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Browse backups in the terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

var pruneData bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the startup service, and optionally every backup",
	Long: `Stops the daemon and removes the OS startup service.

Backups are kept unless you pass --prune or answer yes when asked — deleting
them cannot be undone, so it is never the default.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if running, _ := daemon.IsRunning(); running {
			if err := daemon.Stop(); err == nil {
				done("Daemon stopped")
			}
		}

		if msg, err := daemon.Uninstall(); err != nil {
			note("Could not remove the startup service: %v", err)
		} else {
			done("%s", msg)
		}

		if !pruneData {
			pruneData = confirm(fmt.Sprintf("Delete every backup in %s? This cannot be undone", config.VaultDir()))
		}
		if !pruneData {
			note("Backups kept at %s", config.VaultDir())
			done("envault uninstalled")
			return nil
		}

		if err := os.RemoveAll(config.VaultDir()); err != nil {
			return fmt.Errorf("removing %s: %w", config.VaultDir(), err)
		}
		done("Deleted every backup at %s", config.VaultDir())
		done("envault uninstalled")
		return nil
	},
}

// confirm asks a yes/no question on stdin, defaulting to no.
func confirm(question string) bool {
	fmt.Printf("\n%s [y/N]: ", question)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func init() {
	rootCmd.AddCommand(installCmd, uninstallCmd, uiCmd)
	uninstallCmd.Flags().BoolVar(&pruneData, "prune", false, "Delete all backups without asking")
}
