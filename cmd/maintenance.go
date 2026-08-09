package cmd

import (
	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/format"
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Reclaim disk space by deleting unreferenced content",
	Long: `Deletes stored content that no version points at any more.

Orphans appear when version pruning drops old snapshots (see max_versions in
config.json) or when a file is forgotten. Neither deletes content itself,
because the same bytes may still be shared with another file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		res, err := s.GC()
		if err != nil {
			return err
		}
		if res.Removed == 0 {
			done("Nothing to reclaim — no orphaned content.")
			return nil
		}
		done("Reclaimed %s from %d unreferenced %s",
			format.HumanSize(res.Freed), res.Removed, plural(res.Removed, "blob", "blobs"))
		return nil
	},
}

var forgetCmd = &cobra.Command{
	Use:   "forget <file|#>",
	Short: "Stop tracking a file and delete its history",
	Long: `Removes every recorded version of a file. The file on disk is left alone.

Its stored content survives until the next 'envault gc', so a mistake is
recoverable right up until then.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		absPath, err := resolveFileArg(args[0], s)
		if err != nil {
			return err
		}
		if err := s.Forget(absPath); err != nil {
			return err
		}
		done("Stopped tracking %s", format.ShortenPath(absPath))
		note("Run 'envault gc' to reclaim the disk space.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gcCmd, forgetCmd)
}
