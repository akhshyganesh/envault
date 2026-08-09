package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
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

var (
	pruneData    bool
	uninstallAll bool
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the startup service, and optionally the backups and the binary",
	Long: `Stops the daemon and removes the OS startup service.

With no flags it then asks about the two things that outlive that: your
backups, and the binary itself. Both default to no, because deleting backups
cannot be undone and because removing the service is a reasonable thing to
want on its own.

  envault uninstall           ask about the backups and the binary
  envault uninstall --prune   service and backups; keep the binary
  envault uninstall --all     service, backups and the binary

Whatever survives is named at the end, so 'uninstalled' never means less than
it says.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// A flag means the user already decided; only a bare run asks.
		interactive := !pruneData && !uninstallAll

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

		// Backups.
		removeData := pruneData || uninstallAll
		if interactive {
			removeData = confirm(fmt.Sprintf("Delete every backup in %s? This cannot be undone", config.VaultDir()))
		}
		if removeData {
			if err := os.RemoveAll(config.VaultDir()); err != nil {
				return fmt.Errorf("removing %s: %w", config.VaultDir(), err)
			}
			done("Deleted every backup at %s", config.VaultDir())
		}

		// The binary. Resolving may fail on an exotic setup; that is not a
		// reason to fail the uninstall, only a reason not to offer this.
		exePath, exeErr := resolveExecutable()
		removeBinary := uninstallAll
		if interactive && exeErr == nil {
			removeBinary = confirm(fmt.Sprintf("Remove the binary at %s?", format.ShortenPath(exePath)))
		}
		if removeBinary && exeErr != nil {
			return fmt.Errorf("cannot locate the binary to remove: %w", exeErr)
		}
		if removeBinary {
			if err := removeSelf(exePath); err != nil {
				return err
			}
			done("Removed %s", format.ShortenPath(exePath))
		}

		reportLeftovers(removeData, removeBinary, exePath, exeErr)
		return nil
	},
}

// removeSelf deletes the running binary. Unlinking a file that is currently
// executing is fine on Unix — the kernel keeps the inode alive until this
// process exits — so the only real failure is an install directory owned by
// someone else.
func removeSelf(exePath string) error {
	if err := os.Remove(exePath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf(
				"cannot remove %s: the install directory is not writable by you\n"+
					"  Re-run as: sudo envault uninstall --all\n"+
					"  Or remove it by hand: sudo rm %s",
				exePath, exePath)
		}
		return fmt.Errorf("removing %s: %w", exePath, err)
	}
	return nil
}

// reportLeftovers closes with the truth: either everything is gone, or exactly
// what is left and the command that finishes the job.
func reportLeftovers(dataRemoved, binaryRemoved bool, exePath string, exeErr error) {
	if dataRemoved && binaryRemoved {
		done("envault is fully uninstalled")
		return
	}

	done("Startup service removed")
	if !dataRemoved {
		note("Backups kept at %s", config.VaultDir())
	}
	if !binaryRemoved {
		if exeErr != nil {
			note("The binary is still installed.")
		} else {
			note("The binary is still installed at %s", format.ShortenPath(exePath))
			note("Remove it with: rm %s", exePath)
		}
	}
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
	uninstallCmd.Flags().BoolVar(&pruneData, "prune", false, "Delete all backups too, without asking")
	uninstallCmd.Flags().BoolVar(&uninstallAll, "all", false, "Delete the backups and the binary too, without asking")
}
