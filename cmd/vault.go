package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all tracked .env files",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}
		files, err := s.ListTrackedFiles()
		if err != nil {
			return err
		}
		if len(files) == 0 {
			fmt.Println("No .env files tracked yet. Run 'envault scan' first.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "#\tFILE\tVERSIONS\tLAST BACKUP")
		for i, f := range files {
			last := f.Snapshots[len(f.Snapshots)-1]
			fmt.Fprintf(w, "%d\t%s\t%d\t%s\n",
				i+1,
				format.ShortenPath(f.FilePath),
				len(f.Snapshots),
				format.TimeAgo(last.Timestamp),
			)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Tip: use the # number instead of file path, e.g. 'envault show 3'")
		return w.Flush()
	},
}

var historyCmd = &cobra.Command{
	Use:   "history <file or #>",
	Short: "Show version history for a .env file (accepts index from 'envault list')",
	Args:  cobra.ExactArgs(1),
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
			fmt.Printf("No snapshots found for %s\n", absPath)
			return nil
		}

		fmt.Printf("History for %s (%d versions):\n\n", format.ShortenPath(absPath), len(history.Snapshots))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "#\tID\tDATE\tSIZE\tCOMMENT")
		for i, snap := range history.Snapshots {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
				i+1,
				snap.ID[:12],
				snap.Timestamp.Local().Format("2006-01-02 15:04"),
				format.HumanSize(snap.Size),
				snap.Comment,
			)
		}
		return w.Flush()
	},
}

var showVersion int

var showCmd = &cobra.Command{
	Use:   "show <file or #>",
	Short: "Show the content of a backed-up .env file version (accepts index from 'envault list')",
	Args:  cobra.ExactArgs(1),
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
		if showVersion > 0 {
			if showVersion > len(history.Snapshots) {
				return fmt.Errorf("version %d not found (max: %d)", showVersion, len(history.Snapshots))
			}
			snapIdx = showVersion - 1
		}

		data, err := s.GetBlobContent(history.Snapshots[snapIdx].ID)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(showCmd)
	showCmd.Flags().IntVarP(&showVersion, "version", "v", 0, "Version number to show (default: latest)")
}
