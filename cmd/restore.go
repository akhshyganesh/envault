package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/format"
)

var (
	restoreVersion int
	restoreOutput  string
)

var restoreCmd = &cobra.Command{
	Use:   "restore <file|#>",
	Short: "Restore a .env file from a backup",
	Long: `Writes a stored version back to disk.

Without --version the newest backup is used. Without --output the file is
restored over its original path, so pass --output when you want to compare
before overwriting anything.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		h, err := loadHistory(args[0], s)
		if err != nil {
			return err
		}
		snap, err := pickSnapshot(h.Snapshots, restoreVersion)
		if err != nil {
			return err
		}

		target := h.FilePath
		if restoreOutput != "" {
			if target, err = format.ExpandPath(restoreOutput); err != nil {
				return fmt.Errorf("invalid output path: %w", err)
			}
		}
		if err := s.Restore(target, snap.ID); err != nil {
			return err
		}

		done("Restored %s", format.ShortenPath(target))
		note("Version %s from %s", shortID(snap.ID), snap.Timestamp.Local().Format("2006-01-02 15:04"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().IntVarP(&restoreVersion, "version", "v", 0, "Version to restore (default: latest)")
	restoreCmd.Flags().StringVarP(&restoreOutput, "output", "o", "", "Write here instead of the original path")
}
