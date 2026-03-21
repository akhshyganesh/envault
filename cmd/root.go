package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/akhshy/envault/internal/config"
	"github.com/akhshy/envault/internal/daemon"
	"github.com/akhshy/envault/internal/scanner"
	"github.com/akhshy/envault/internal/store"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "envault",
	Short: "🔒 envault — your .env files, safely vaulted",
	Long: `envault automatically discovers, versions, and backs up all .env files
on your machine. It runs as a background daemon and lets you restore
any previous version with a single command.

Works on macOS and Ubuntu.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
}

// ── envault init ──────────────────────────────────────────────────────────────

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize envault (creates config and vault directory)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("✓ envault initialized at %s\n", config.VaultDir())
		fmt.Printf("  Config: %s\n", config.ConfigPath())
		fmt.Printf("  Watching: %s\n", strings.Join(cfg.WatchDirs, ", "))
		fmt.Printf("\nRun 'envault scan' to do your first backup, or 'envault start' to run the daemon.\n")
		return nil
	},
}

// ── envault scan ──────────────────────────────────────────────────────────────

var scanDirs []string

var scanCmd = &cobra.Command{
	Use:   "scan [directories...]",
	Short: "Scan directories for .env files and back them up",
	Long:  "Walks the given directories (or configured watch dirs) and creates versioned backups of all .env files found.",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
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

		fmt.Printf("Scanning %d directory(ies) for .env files...\n", len(dirs))
		result, err := scanner.ScanDirectories(dirs, s)
		if err != nil {
			return err
		}

		fmt.Printf("\n✓ Found %d .env file(s)\n", len(result.Found))
		fmt.Printf("  New snapshots: %d\n", result.Backed)
		fmt.Printf("  Unchanged:     %d\n", result.Skipped)
		if len(result.Errors) > 0 {
			fmt.Printf("  Warnings:      %d\n", len(result.Errors))
		}
		return nil
	},
}

// ── envault list ──────────────────────────────────────────────────────────────

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked .env files",
	Aliases: []string{"ls"},
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
		fmt.Fprintln(w, "FILE\tVERSIONS\tLAST BACKUP")
		for _, f := range files {
			last := f.Snapshots[len(f.Snapshots)-1]
			fmt.Fprintf(w, "%s\t%d\t%s\n",
				shortenPath(f.FilePath),
				len(f.Snapshots),
				timeAgo(last.Timestamp),
			)
		}
		return w.Flush()
	},
}

// ── envault history ───────────────────────────────────────────────────────────

var historyCmd = &cobra.Command{
	Use:   "history <file>",
	Short: "Show version history for a .env file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}

		absPath, err := filepath.Abs(args[0])
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

		fmt.Printf("History for %s (%d versions):\n\n", shortenPath(absPath), len(history.Snapshots))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "#\tID\tDATE\tSIZE\tCOMMENT")
		for i, snap := range history.Snapshots {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
				i+1,
				snap.ID[:12],
				snap.Timestamp.Local().Format("2006-01-02 15:04"),
				humanSize(snap.Size),
				snap.Comment,
			)
		}
		return w.Flush()
	},
}

// ── envault restore ───────────────────────────────────────────────────────────

var restoreVersion int

var restoreCmd = &cobra.Command{
	Use:   "restore <file>",
	Short: "Restore a .env file from a backup",
	Long:  "Restores a .env file to a previous version. Use --version to pick a specific version (default: latest).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}

		absPath, err := filepath.Abs(args[0])
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

		var snap store.Snapshot
		if restoreVersion <= 0 {
			snap = history.Snapshots[len(history.Snapshots)-1]
		} else if restoreVersion > len(history.Snapshots) {
			return fmt.Errorf("version %d not found (max: %d)", restoreVersion, len(history.Snapshots))
		} else {
			snap = history.Snapshots[restoreVersion-1]
		}

		if err := s.RestoreSnapshot(absPath, snap.ID); err != nil {
			return err
		}

		fmt.Printf("✓ Restored %s to version %s (%s)\n",
			shortenPath(absPath),
			snap.ID[:12],
			snap.Timestamp.Local().Format("2006-01-02 15:04"),
		)
		return nil
	},
}

func init() {
	restoreCmd.Flags().IntVarP(&restoreVersion, "version", "v", 0, "Version number to restore (default: latest)")
}

// ── envault show ──────────────────────────────────────────────────────────────

var showVersion int

var showCmd = &cobra.Command{
	Use:   "show <file>",
	Short: "Show the content of a backed-up .env file version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.NewStore()
		if err != nil {
			return err
		}

		absPath, err := filepath.Abs(args[0])
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

		var snap store.Snapshot
		if showVersion <= 0 {
			snap = history.Snapshots[len(history.Snapshots)-1]
		} else if showVersion > len(history.Snapshots) {
			return fmt.Errorf("version %d not found (max: %d)", showVersion, len(history.Snapshots))
		} else {
			snap = history.Snapshots[showVersion-1]
		}

		data, err := s.GetBlobContent(snap.ID)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	},
}

func init() {
	showCmd.Flags().IntVarP(&showVersion, "version", "v", 0, "Version number to show (default: latest)")
}

// ── envault start / stop / status ─────────────────────────────────────────────

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the envault background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Run()
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the envault background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Stop()
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the envault daemon is running",
	Run: func(cmd *cobra.Command, args []string) {
		running, pid := daemon.IsRunning()
		if running {
			fmt.Printf("envault daemon is running (PID %d)\n", pid)
		} else {
			fmt.Println("envault daemon is not running")
		}
	},
}

// ── envault watch ─────────────────────────────────────────────────────────────

var watchCmd = &cobra.Command{
	Use:   "watch <directory>",
	Short: "Add a directory to the watch list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absDir, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		info, err := os.Stat(absDir)
		if err != nil {
			return fmt.Errorf("directory not found: %s", absDir)
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", absDir)
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// Check for duplicate
		for _, d := range cfg.WatchDirs {
			if d == absDir {
				fmt.Printf("Already watching %s\n", absDir)
				return nil
			}
		}

		cfg.WatchDirs = append(cfg.WatchDirs, absDir)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("✓ Now watching %s\n", absDir)
		return nil
	},
}

// ── envault install / uninstall ───────────────────────────────────────────────

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install envault as a startup service (launchd on macOS, systemd on Linux)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Install()
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove envault startup service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Uninstall()
	},
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func humanSize(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%dB", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	}
}
