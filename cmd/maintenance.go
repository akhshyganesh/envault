package cmd

import (
	"fmt"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/spf13/cobra"
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Reclaim disk space by deleting unreferenced backup blobs",
	Long: `Removes content blobs that are no longer referenced by any tracked file.
Orphans accumulate when version pruning trims old snapshots (see max_versions
in config.json) or when files are forgotten with 'envault forget'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}
		res, err := s.GC()
		if err != nil {
			return err
		}
		if res.Removed == 0 {
			fmt.Println("✓ Nothing to clean — no orphaned blobs.")
			return nil
		}
		fmt.Printf("✓ Removed %d orphaned blob(s), reclaimed %s\n", res.Removed, format.HumanSize(res.Freed))
		return nil
	},
}

var forgetCmd = &cobra.Command{
	Use:   "forget <file or #>",
	Short: "Stop tracking a file and delete its version history (accepts index from 'envault list')",
	Long: `Removes all recorded version history for a file. The underlying content blobs
are kept until the next 'envault gc' run, since they may be shared with other files.`,
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
		if err := s.Forget(absPath); err != nil {
			return err
		}
		fmt.Printf("✓ Forgot %s — run 'envault gc' to reclaim disk space\n", format.ShortenPath(absPath))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gcCmd)
	rootCmd.AddCommand(forgetCmd)
}
