package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/setup"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Interactive setup — watch directories, scan interval, and startup service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return setup.Run()
	},
}

var pruneData bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove envault startup service and optionally delete all data",
	RunE: func(cmd *cobra.Command, args []string) error {
		if running, _ := daemon.IsRunning(); running {
			// Best-effort; Stop() prints its own confirmation.
			_ = daemon.Stop()
		}

		if err := daemon.Uninstall(); err != nil {
			fmt.Printf("⚠ Service removal: %v\n", err)
		}

		if !pruneData {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("\nDelete all envault data (~/.envault)? This removes all backups. [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			pruneData = answer == "y" || answer == "yes"
		}

		if pruneData {
			vaultDir := config.VaultDir()
			if err := os.RemoveAll(vaultDir); err != nil {
				return fmt.Errorf("failed to remove %s: %w", vaultDir, err)
			}
			fmt.Printf("✓ Deleted all data at %s\n", vaultDir)
		} else {
			fmt.Printf("  Backups preserved at %s\n", config.VaultDir())
		}

		fmt.Println("✓ envault fully uninstalled")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolVar(&pruneData, "prune", false, "Delete all envault data including backups")
}
