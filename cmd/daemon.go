package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Run the background daemon in the foreground",
	Long: `Scans the watch directories on a timer until stopped with Ctrl+C.

The OS startup service installed by 'envault install' runs exactly this.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Printf("envault daemon running — checking every %ds. Ctrl+C to stop.\n", cfg.ScanIntervalSecs)
		if err := daemon.Run(); err != nil {
			return err
		}
		fmt.Println("\nenvault daemon stopped")
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, pid := daemon.IsRunning()
		if err := daemon.Stop(); err != nil {
			return err
		}
		done("Daemon stopped (PID %d)", pid)
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the daemon is running",
	Run: func(cmd *cobra.Command, args []string) {
		if running, pid := daemon.IsRunning(); running {
			fmt.Printf("envault daemon is running (PID %d)\n", pid)
		} else {
			fmt.Println("envault daemon is not running")
		}
	},
}

var watchCmd = &cobra.Command{
	Use:   "watch <directory>",
	Short: "Add a directory to the watch list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := format.ExpandPath(args[0])
		if err != nil {
			return err
		}
		if _, err := config.AddWatchDir(dir); err != nil {
			return err
		}
		done("Now watching %s", format.ShortenPath(dir))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd, stopCmd, statusCmd, watchCmd)
}
