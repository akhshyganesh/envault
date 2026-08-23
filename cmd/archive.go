package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
)

// resolveArchivedArg accepts a number from 'envault list --archived' or a
// path, mirroring resolveFileArg for the archive shelf.
func resolveArchivedArg(arg string, s *store.Store) (string, error) {
	if idx, err := strconv.Atoi(arg); err == nil {
		files, err := s.ListArchived()
		if err != nil {
			return "", err
		}
		if idx < 1 || idx > len(files) {
			return "", fmt.Errorf("index %d out of range (1-%d) — run 'envault list --archived' to see them", idx, len(files))
		}
		return files[idx-1].FilePath, nil
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", err
	}
	files, err := s.ListArchived()
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.FilePath == abs {
			return abs, nil
		}
	}
	return "", fmt.Errorf("%s is not archived — see 'envault list --archived'", format.ShortenPath(abs))
}

var archiveCmd = &cobra.Command{
	Use:   "archive <file|#>",
	Short: "Park a file's backups outside the live index",
	Long: `Moves a file's recorded history into the vault's archive folder.

An archived file stops appearing in lists and is no longer picked up by scans
or the daemon, but every version stays recoverable and safe from 'envault gc'.
Bring it back with 'envault unarchive'.`,
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
		if err := s.Archive(absPath); err != nil {
			return err
		}
		done("Archived %s", format.ShortenPath(absPath))
		note("It will not be scanned again until you run 'envault unarchive %s'.", args[0])
		return nil
	},
}

var unarchiveCmd = &cobra.Command{
	Use:   "unarchive <file|#>",
	Short: "Bring an archived file's history back",
	Long: `Returns an archived file's history to the live index.

If the file was scanned again while it was parked, the two histories are
merged, so no versions are lost.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		absPath, err := resolveArchivedArg(args[0], s)
		if err != nil {
			return err
		}
		if err := s.Unarchive(absPath); err != nil {
			return err
		}
		done("Brought back %s", format.ShortenPath(absPath))
		note("Run 'envault scan' to pick up where it left off.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(archiveCmd, unarchiveCmd)
	listCmd.Flags().BoolVar(&listArchived, "archived", false, "List archived files instead of tracked ones")
}
