package cmd

import (
	"fmt"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the envault background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Run()
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the envault background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Stop()
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the envault daemon is running",
	Run: func(cmd *cobra.Command, args []string) {
		running, pid := daemon.IsRunning()
		if running {
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
		absDir, err := format.ExpandPath(args[0])
		if err != nil {
			return err
		}

		_, err = config.AddWatchDir(absDir)
		if err != nil {
			return err
		}
		fmt.Printf("✓ Now watching %s\n", format.ShortenPath(absDir))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(watchCmd)
}
