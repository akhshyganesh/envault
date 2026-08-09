package cmd

import (
	"fmt"
	"strings"

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

// statusCmd answers three separate questions, because a running daemon says
// nothing about whether envault survives a reboot — and someone who ran
// 'envault start' by hand would otherwise be told everything is fine.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report the daemon, the login service and the watch list",
	Run: func(cmd *cobra.Command, args []string) {
		running, pid := daemon.IsRunning()
		if running {
			done("Daemon running (PID %d)", pid)
		} else {
			note("Daemon not running")
		}

		installed, path := daemon.ServiceInstalled()
		if installed {
			done("Starts at login (%s)", format.ShortenPath(path))
		} else {
			note("Will not start at login")
		}

		if !config.IsConfigured() {
			note("No watch list — envault is scanning your whole home directory")
		} else if cfg, err := config.Load(); err == nil {
			dirs := make([]string, len(cfg.WatchDirs))
			for i, d := range cfg.WatchDirs {
				dirs[i] = format.ShortenPath(d)
			}
			done("Watching %s every %ds", strings.Join(dirs, ", "), cfg.ScanIntervalSecs)
		}

		if !installed || !config.IsConfigured() {
			fmt.Println("\nRun 'envault install' to finish setup.")
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
