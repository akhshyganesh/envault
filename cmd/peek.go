// ABOUTME: 'envault peek' — browse an exported vault zip without importing it.
// ABOUTME: Lists files, shows history/content, and extracts single versions.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
	"github.com/spf13/cobra"
)

var (
	peekVersion int
	peekShow    bool
	peekOutput  string
	peekForce   bool
)

var peekCmd = &cobra.Command{
	Use:   "peek <zipfile> [file or #]",
	Short: "Browse an exported zip read-only, without importing it",
	Long: `Inspects a zip created by 'envault export' without writing anything to
this machine's vault. Use it to check what a backup contains, or to pull a
single .env file out of it.

  envault peek backup.zip                    list the files in the archive
  envault peek backup.zip 3                  version history for file #3
  envault peek backup.zip 3 --show           print the latest version
  envault peek backup.zip 3 --show -v 2      print version 2
  envault peek backup.zip 3 -o ./.env        write a version to a path

<file or #> accepts the number from the archive listing, a full path, or an
unambiguous path suffix such as 'myproject/.env'.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := transfer.OpenArchive(args[0])
		if err != nil {
			return err
		}
		defer a.Close()

		if len(args) == 1 {
			if peekShow || peekOutput != "" {
				return fmt.Errorf("specify which file to read, e.g. 'envault peek %s 1 --show'", args[0])
			}
			return printArchiveListing(a)
		}

		history, err := a.Resolve(args[1])
		if err != nil {
			return err
		}
		if len(history.Snapshots) == 0 {
			return fmt.Errorf("no versions stored for %s in this archive", history.FilePath)
		}

		snap, err := pickSnapshot(history.Snapshots, peekVersion)
		if err != nil {
			return err
		}

		switch {
		case peekOutput != "":
			return extractSnapshot(a, snap, peekOutput)
		case peekShow:
			data, err := a.Content(snap.ID)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		default:
			return printArchiveHistory(history)
		}
	},
}

// printArchiveListing prints the tracked files in an archive, mirroring 'envault list'.
func printArchiveListing(a *transfer.Archive) error {
	files := a.Files()
	if len(files) == 0 {
		fmt.Println("This archive contains no tracked .env files.")
		return nil
	}

	if cfg, err := a.Config(); err == nil {
		fmt.Printf("Archive %s — %d files, watching %d dir(s)\n\n",
			filepath.Base(a.Path), len(files), len(cfg.WatchDirs))
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tFILE\tVERSIONS\tLAST BACKUP")
	for i, f := range files {
		versions, last := len(f.Snapshots), "—"
		if versions > 0 {
			last = format.TimeAgo(f.Snapshots[versions-1].Timestamp)
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\n", i+1, format.ShortenPath(f.FilePath), versions, last)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Nothing was written to your vault. Add a # to inspect a file, e.g. 'envault peek <zip> 1'")
	return w.Flush()
}

// printArchiveHistory prints a file's versions inside an archive, mirroring 'envault history'.
func printArchiveHistory(h *store.FileHistory) error {
	fmt.Printf("History for %s in archive (%d versions):\n\n",
		format.ShortenPath(h.FilePath), len(h.Snapshots))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tID\tDATE\tSIZE\tCOMMENT")
	for i, snap := range h.Snapshots {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			i+1,
			snap.ID[:12],
			snap.Timestamp.Local().Format("2006-01-02 15:04"),
			format.HumanSize(snap.Size),
			snap.Comment,
		)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Add --show to print a version, or -o <path> to write one out.")
	return w.Flush()
}

// pickSnapshot selects a 1-based version, defaulting to the latest.
func pickSnapshot(snapshots []store.Snapshot, version int) (*store.Snapshot, error) {
	if version <= 0 {
		return &snapshots[len(snapshots)-1], nil
	}
	if version > len(snapshots) {
		return nil, fmt.Errorf("version %d not found (max: %d)", version, len(snapshots))
	}
	return &snapshots[version-1], nil
}

// extractSnapshot writes one archived version to a path outside the vault.
func extractSnapshot(a *transfer.Archive, snap *store.Snapshot, outPath string) error {
	data, err := a.Content(snap.ID)
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(abs); statErr == nil && !peekForce {
		return fmt.Errorf("%s already exists — use --force to overwrite", abs)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(abs, data, 0600); err != nil {
		return err
	}

	fmt.Printf("✓ Wrote %s (%s) to %s\n", format.ShortenPath(snap.FilePath), format.HumanSize(snap.Size), abs)
	fmt.Println("  Your vault was not modified.")
	return nil
}

func init() {
	rootCmd.AddCommand(peekCmd)
	peekCmd.Flags().IntVarP(&peekVersion, "version", "v", 0, "Version number to read (default: latest)")
	peekCmd.Flags().BoolVar(&peekShow, "show", false, "Print the version's content to stdout")
	peekCmd.Flags().StringVarP(&peekOutput, "out", "o", "", "Write the version to this path instead of printing it")
	peekCmd.Flags().BoolVar(&peekForce, "force", false, "Overwrite the output path if it exists")
}
