package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
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
		absDir, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		info, err := os.Stat(absDir)
		if err != nil {
			return fmt.Errorf("directory not found: %s", absDir)
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", absDir)
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		for _, d := range cfg.WatchDirs {
			if d == absDir {
				fmt.Printf("Already watching %s\n", absDir)
				return nil
			}
		}

		cfg.WatchDirs = append(cfg.WatchDirs, absDir)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("✓ Now watching %s\n", absDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(watchCmd)
}
