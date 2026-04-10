package cmd

import (
	"fmt"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan [directories...]",
	Short: "Scan directories for .env files and back them up",
	Long:  "Walks the given directories (or configured watch dirs) and creates versioned backups of all .env files found.",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}

		dirs := args
		if len(dirs) == 0 {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dirs = cfg.WatchDirs
		}

		fmt.Printf("Scanning %d directory(ies) for .env files...\n", len(dirs))
		result, err := scanner.ScanDirectories(dirs, s)
		if err != nil {
			return err
		}

		fmt.Printf("\n✓ Found %d .env file(s)\n", len(result.Found))
		fmt.Printf("  New snapshots: %d\n", result.Backed)
		fmt.Printf("  Unchanged:     %d\n", result.Skipped)
		if len(result.Errors) > 0 {
			fmt.Printf("  Warnings:      %d\n", len(result.Errors))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
