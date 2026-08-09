package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
)

var (
	peekVersion int
	peekShow    bool
	peekOut     string
	peekForce   bool
)

// peek is deliberately the one command that reads a vault it does not own. It
// never writes to ~/.envault — pulling something out of a backup should not
// require importing the whole thing over what is already there.
var peekCmd = &cobra.Command{
	Use:   "peek <file.zip> [file|#]",
	Short: "Browse an exported zip read-only, without importing it",
	Long: `Inspects a zip made by 'envault export' without touching this machine's vault.

  envault peek backup.zip                    what is inside
  envault peek backup.zip 3                  version history for file #3
  envault peek backup.zip 3 --show           print the newest version
  envault peek backup.zip 3 --show -v 2      print version 2
  envault peek backup.zip 3 -o ./.env        write a version out

<file|#> takes the number from the listing, a full path, or an unambiguous
suffix such as 'myproject/.env'. Note the output flag here is --out, because
--output would read like it writes the whole archive.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := transfer.OpenArchive(args[0])
		if err != nil {
			return err
		}
		defer a.Close()

		if len(args) == 1 {
			if peekShow || peekOut != "" {
				return fmt.Errorf("say which file to read, e.g. 'envault peek %s 1 --show'", args[0])
			}
			return printArchive(a)
		}

		h, err := a.Resolve(args[1])
		if err != nil {
			return err
		}
		if len(h.Snapshots) == 0 {
			return fmt.Errorf("no versions stored for %s in this archive", h.FilePath)
		}
		snap, err := pickSnapshot(h.Snapshots, peekVersion)
		if err != nil {
			return err
		}

		switch {
		case peekOut != "":
			return extractFromArchive(a, snap, peekOut)
		case peekShow:
			data, err := a.Content(snap.ID)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		default:
			return printArchiveHistory(h)
		}
	},
}

func printArchive(a *transfer.Archive) error {
	files := a.Files()
	if len(files) == 0 {
		fmt.Println("This archive contains no tracked .env files.")
		return nil
	}

	if cfg, err := a.Config(); err == nil {
		fmt.Printf("%s — %d %s, watching %d %s\n\n",
			filepath.Base(a.Path),
			len(files), plural(len(files), "file", "files"),
			len(cfg.WatchDirs), plural(len(cfg.WatchDirs), "directory", "directories"))
	}

	w := table()
	fmt.Fprintln(w, "#\tFILE\tVERSIONS\tLAST BACKUP")
	for i, f := range files {
		last := "—"
		if latest := f.Latest(); latest != nil {
			last = format.TimeAgo(latest.Timestamp)
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\n", i+1, format.ShortenPath(f.FilePath), len(f.Snapshots), last)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Your vault was not touched. Add a # to inspect one, e.g. 'envault peek <zip> 1'")
	return w.Flush()
}

func printArchiveHistory(h *store.FileHistory) error {
	fmt.Printf("%s in archive — %d %s\n\n",
		format.ShortenPath(h.FilePath), len(h.Snapshots), plural(len(h.Snapshots), "version", "versions"))

	w := table()
	printVersions(w, h.Snapshots)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Add --show to print a version, or -o <path> to write one out.")
	return w.Flush()
}

// extractFromArchive writes one archived version out. Every reason to refuse is
// checked before the content is read, so a rejected call does no work at all.
func extractFromArchive(a *transfer.Archive, snap *store.Snapshot, outPath string) error {
	abs, err := format.ExpandPath(outPath)
	if err != nil {
		return err
	}
	// peek promises it never touches the vault, and says so on success.
	if config.IsInsideVault(abs) {
		return fmt.Errorf(
			"refusing to write into the vault at %s — peek is read-only\n"+
				"  use 'envault import' to load an archive", config.VaultDir())
	}
	if _, err := os.Stat(abs); err == nil && !peekForce {
		return fmt.Errorf("%s already exists — pass --force to overwrite it", abs)
	}

	data, err := a.Content(snap.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(abs, data, 0600); err != nil {
		return err
	}

	done("Wrote %s (%s) to %s", format.ShortenPath(snap.FilePath), format.HumanSize(snap.Size), abs)
	note("Your vault was not modified.")
	return nil
}

func init() {
	rootCmd.AddCommand(peekCmd)
	peekCmd.Flags().IntVarP(&peekVersion, "version", "v", 0, "Version to read (default: latest)")
	peekCmd.Flags().BoolVar(&peekShow, "show", false, "Print the version to stdout")
	peekCmd.Flags().StringVarP(&peekOut, "out", "o", "", "Write the version to this path")
	peekCmd.Flags().BoolVar(&peekForce, "force", false, "Overwrite the output path if it exists")
}
