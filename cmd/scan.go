package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/scanner"
)

var scanCmd = &cobra.Command{
	Use:   "scan [directories...]",
	Short: "Find .env files and back them up",
	Long: `Walks the given directories — or the configured watch directories when
none are given — and records a new version of every .env file whose content
has changed since the last scan.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
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

		fmt.Printf("Scanning %d %s…\n", len(dirs), plural(len(dirs), "directory", "directories"))
		res, err := scanner.ScanDirectories(dirs, s)
		if err != nil {
			return err
		}

		fmt.Println()
		done("Found %d .env %s", len(res.Found), plural(len(res.Found), "file", "files"))
		note("New versions: %d", res.Backed)
		note("Unchanged:    %d", res.Skipped)
		if len(res.Errors) > 0 {
			note("Skipped:      %d (unreadable paths)", len(res.Errors))
		}
		return nil
	},
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
