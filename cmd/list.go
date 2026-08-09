package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/format"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List every tracked .env file",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		files, err := s.ListTrackedFiles()
		if err != nil {
			return err
		}
		if len(files) == 0 {
			fmt.Println("Nothing tracked yet. Run 'envault scan' to take your first backup.")
			return nil
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
		fmt.Fprintln(w, "Use the # anywhere a file is expected, e.g. 'envault show 3'")
		return w.Flush()
	},
}

var historyCmd = &cobra.Command{
	Use:   "history <file|#>",
	Short: "Show a file's version history",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		h, err := loadHistory(args[0], s)
		if err != nil {
			return err
		}

		fmt.Printf("%s — %d %s\n\n",
			format.ShortenPath(h.FilePath), len(h.Snapshots), plural(len(h.Snapshots), "version", "versions"))
		w := table()
		printVersions(w, h.Snapshots)
		return w.Flush()
	},
}

var showVersion int

var showCmd = &cobra.Command{
	Use:   "show <file|#>",
	Short: "Print a backed-up version to stdout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		h, err := loadHistory(args[0], s)
		if err != nil {
			return err
		}
		snap, err := pickSnapshot(h.Snapshots, showVersion)
		if err != nil {
			return err
		}
		data, err := s.Content(snap.ID)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd, historyCmd, showCmd)
	showCmd.Flags().IntVarP(&showVersion, "version", "v", 0, "Version to print (default: latest)")
}
