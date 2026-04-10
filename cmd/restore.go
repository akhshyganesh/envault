package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/spf13/cobra"
)

var restoreVersion int
var restoreOutput string

var restoreCmd = &cobra.Command{
	Use:   "restore <file or #>",
	Short: "Restore a .env file from a backup (accepts index from 'envault list')",
	Long: `Restores a .env file to a previous version.
Use --version to pick a specific version (default: latest).
Use --output to write to a custom path instead of the original location.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}

		absPath, err := resolveFileArg(args[0], s)
		if err != nil {
			return err
		}

		history, err := s.GetHistory(absPath)
		if err != nil {
			return err
		}
		if len(history.Snapshots) == 0 {
			return fmt.Errorf("no snapshots found for %s", absPath)
		}

		snapIdx := len(history.Snapshots) - 1
		if restoreVersion > 0 {
			if restoreVersion > len(history.Snapshots) {
				return fmt.Errorf("version %d not found (max: %d)", restoreVersion, len(history.Snapshots))
			}
			snapIdx = restoreVersion - 1
		}
		snap := history.Snapshots[snapIdx]

		targetPath := absPath
		if restoreOutput != "" {
			targetPath, err = filepath.Abs(restoreOutput)
			if err != nil {
				return fmt.Errorf("invalid output path: %w", err)
			}
		}

		if err := s.RestoreSnapshot(targetPath, snap.ID); err != nil {
			return err
		}

		fmt.Printf("✓ Restored %s to version %s (%s)\n",
			format.ShortenPath(targetPath),
			snap.ID[:12],
			snap.Timestamp.Local().Format("2006-01-02 15:04"),
		)
		if restoreOutput != "" {
			fmt.Printf("  Written to: %s\n", targetPath)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().IntVarP(&restoreVersion, "version", "v", 0, "Version number to restore (default: latest)")
	restoreCmd.Flags().StringVarP(&restoreOutput, "output", "o", "", "Write restored file to this path (default: original location)")
}
