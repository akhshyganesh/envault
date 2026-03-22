package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/tui"
	"github.com/spf13/cobra"
)

// Version info — set via ldflags at build time.
var (
	Version   = "dev"
	BuildDate = "unknown"
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
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("envault {{.Version}}\n")
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
	rootCmd.AddCommand(uiCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)
}

// ── envault init ──────────────────────────────────────────────────────────────

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize envault (creates config and vault directory)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detect first-time install (vault dir doesn't exist yet)
		firstTime := false
		if _, err := os.Stat(config.VaultDir()); os.IsNotExist(err) {
			firstTime = true
		}

		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return err
		}

		if firstTime {
			fmt.Println()
			fmt.Println("  ╔══════════════════════════════════════════════════╗")
			fmt.Println("  ║     🔒 Welcome to envault!                      ║")
			fmt.Println("  ║     Your .env files, safely vaulted.             ║")
			fmt.Println("  ╚══════════════════════════════════════════════════╝")
			fmt.Println()
			fmt.Printf("  ✓ Vault created at %s\n", config.VaultDir())
			fmt.Printf("  ✓ Config: %s\n", config.ConfigPath())
			fmt.Printf("  ✓ Watching: %s\n", strings.Join(cfg.WatchDirs, ", "))
			fmt.Println()
			fmt.Println("  ── Get Started ──────────────────────────────────")
			fmt.Println("  Run 'envault scan' to do your first backup")
			fmt.Println("  Run 'envault start' to launch the background daemon")
			fmt.Println()
			fmt.Println("  ── Support the Project ──────────────────────────")
			fmt.Println("  ⭐ Star us on GitHub: https://github.com/akhshyganesh/envault")
			fmt.Println("  📺 Watch tutorials:   https://www.youtube.com/@code_wid_mapla")
			fmt.Println()
			fmt.Println("  Made with ❤ by Akhshy (Code_Wid_Mapla)")
			fmt.Println()
		} else {
			fmt.Printf("✓ envault re-initialized at %s\n", config.VaultDir())
			fmt.Printf("  Config: %s\n", config.ConfigPath())
			fmt.Printf("  Watching: %s\n", strings.Join(cfg.WatchDirs, ", "))
			fmt.Printf("\nRun 'envault scan' to do your first backup, or 'envault start' to run the daemon.\n")
		}
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
		fmt.Fprintln(w, "#\tFILE\tVERSIONS\tLAST BACKUP")
		for i, f := range files {
			last := f.Snapshots[len(f.Snapshots)-1]
			fmt.Fprintf(w, "%d\t%s\t%d\t%s\n",
				i+1,
				shortenPath(f.FilePath),
				len(f.Snapshots),
				timeAgo(last.Timestamp),
			)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Tip: use the # number instead of file path, e.g. 'envault show 3'")
		return w.Flush()
	},
}

// ── envault history ───────────────────────────────────────────────────────────

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
var restoreOutput string

var restoreCmd = &cobra.Command{
	Use:   "restore <file or #>",
	Short: "Restore a .env file from a backup (accepts index from 'envault list')",
	Long: `Restores a .env file to a previous version.
Use --version to pick a specific version (default: latest).
Use --output to write the restored file to a custom path instead of the original location.`,
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

		var snap store.Snapshot
		if restoreVersion <= 0 {
			snap = history.Snapshots[len(history.Snapshots)-1]
		} else if restoreVersion > len(history.Snapshots) {
			return fmt.Errorf("version %d not found (max: %d)", restoreVersion, len(history.Snapshots))
		} else {
			snap = history.Snapshots[restoreVersion-1]
		}

		// Determine the target path: --output overrides the original location
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
			shortenPath(targetPath),
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
	restoreCmd.Flags().IntVarP(&restoreVersion, "version", "v", 0, "Version number to restore (default: latest)")
	restoreCmd.Flags().StringVarP(&restoreOutput, "output", "o", "", "Output path to write restored file (default: original location)")
}

// ── envault show ──────────────────────────────────────────────────────────────

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
		reader := bufio.NewReader(os.Stdin)
		var watchDirs []string

		fmt.Println("Which directories should envault watch for .env files?")
		fmt.Println("Enter one directory per line. Press Enter on an empty line when done.")
		fmt.Println()

		for i := 1; ; i++ {
			fmt.Printf("  [%d] Directory path (or Enter to finish): ", i)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)

			if line == "" {
				break
			}

			// Expand ~ to home dir
			if strings.HasPrefix(line, "~/") {
				home, _ := os.UserHomeDir()
				line = filepath.Join(home, line[2:])
			}

			absDir, err := filepath.Abs(line)
			if err != nil {
				fmt.Printf("    ⚠ Invalid path: %s\n", err)
				i--
				continue
			}

			info, err := os.Stat(absDir)
			if err != nil || !info.IsDir() {
				fmt.Printf("    ⚠ Not a valid directory: %s\n", absDir)
				i--
				continue
			}

			watchDirs = append(watchDirs, absDir)
			fmt.Printf("    ✓ Added %s\n", absDir)
		}

		if len(watchDirs) == 0 {
			home, _ := os.UserHomeDir()
			watchDirs = []string{home}
			fmt.Printf("\n  No directories specified, defaulting to %s\n", home)
		}

		cfg, err := config.Load()
		if err != nil {
			cfg = config.DefaultConfig()
		}
		cfg.WatchDirs = watchDirs
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		fmt.Println()
		fmt.Printf("Watching %d directory(ies):\n", len(watchDirs))
		for _, d := range watchDirs {
			fmt.Printf("  • %s\n", d)
		}
		fmt.Println()

		return daemon.Install()
	},
}

var pruneData bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove envault startup service and optionally delete all data",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Stop daemon if running
		if running, _ := daemon.IsRunning(); running {
			fmt.Println("Stopping running daemon...")
			daemon.Stop()
		}

		// Remove the OS service
		if err := daemon.Uninstall(); err != nil {
			fmt.Printf("⚠ Service removal: %v\n", err)
		}

		if !pruneData {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("\nDelete all envault data (~/.envault)? This removes all backups. [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			pruneData = answer == "y" || answer == "yes"
		}

		if pruneData {
			vaultDir := config.VaultDir()
			if err := os.RemoveAll(vaultDir); err != nil {
				return fmt.Errorf("failed to remove %s: %w", vaultDir, err)
			}
			fmt.Printf("✓ Deleted all data at %s\n", vaultDir)
		} else {
			fmt.Printf("  Backups preserved at %s\n", config.VaultDir())
		}

		fmt.Println("✓ envault fully uninstalled")
		return nil
	},
}

func init() {
	uninstallCmd.Flags().BoolVar(&pruneData, "prune", false, "Delete all envault data including backups")
}

// ── envault version ───────────────────────────────────────────────────────────

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print envault version and credits",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🔒 envault — your .env files, safely vaulted")
		fmt.Println()
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Built:      %s\n", BuildDate)
		fmt.Println()
		fmt.Println("  Created by: Akhshy (Code_Wid_Mapla)")
		fmt.Println("  GitHub:     https://github.com/akhshyganesh/envault")
		fmt.Println("  YouTube:    https://www.youtube.com/@code_wid_mapla")
		fmt.Println()
	},
}

// ── envault upgrade ───────────────────────────────────────────────────────────

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade envault to the latest release from GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Checking for latest version...")

		// Fetch latest release tag from GitHub API
		latestVersion, err := getLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to check latest version: %w", err)
		}

		if latestVersion == Version {
			fmt.Printf("✓ Already on the latest version (%s)\n", Version)
			return nil
		}

		fmt.Printf("  Current: %s\n", Version)
		fmt.Printf("  Latest:  %s\n", latestVersion)
		fmt.Println()

		// Determine asset name based on OS/arch
		assetName := fmt.Sprintf("envault-%s-%s", runtime.GOOS, runtime.GOARCH)
		downloadURL := fmt.Sprintf("https://github.com/akhshyganesh/envault/releases/latest/download/%s", assetName)

		fmt.Printf("Downloading %s...\n", assetName)

		// Download to a temp file
		resp, err := http.Get(downloadURL)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed: HTTP %d (no release binary found for %s)", resp.StatusCode, assetName)
		}

		// Find current binary path
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine binary path: %w", err)
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			return fmt.Errorf("cannot resolve binary path: %w", err)
		}

		// Write to temp file in the same directory (for atomic rename)
		tmpFile, err := os.CreateTemp(filepath.Dir(exePath), "envault-upgrade-*")
		if err != nil {
			return fmt.Errorf("cannot create temp file: %w (try running with sudo)", err)
		}
		tmpPath := tmpFile.Name()

		_, err = io.Copy(tmpFile, resp.Body)
		tmpFile.Close()
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("download interrupted: %w", err)
		}

		// Make executable
		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("cannot set permissions: %w", err)
		}

		// Atomic replace
		if err := os.Rename(tmpPath, exePath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("cannot replace binary: %w (try running with sudo)", err)
		}

		fmt.Printf("\n✓ Upgraded envault to %s\n", latestVersion)
		fmt.Println("  Run 'envault version' to verify.")
		return nil
	},
}

// getLatestVersion fetches the latest release tag from GitHub.
func getLatestVersion() (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}
	resp, err := client.Get("https://api.github.com/repos/akhshyganesh/envault/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// ── envault ui ────────────────────────────────────────────────────────────────

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch interactive TUI to browse files, versions, and content",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// resolveFileArg takes a CLI argument that is either a file path or a
// numeric index (from 'envault list') and returns the absolute file path.
func resolveFileArg(arg string, s *store.Store) (string, error) {
	// Try parsing as a number first
	if idx, err := strconv.Atoi(arg); err == nil {
		files, err := s.ListTrackedFiles()
		if err != nil {
			return "", err
		}
		if idx < 1 || idx > len(files) {
			return "", fmt.Errorf("index %d out of range (1-%d). Run 'envault list' to see indices", idx, len(files))
		}
		return files[idx-1].FilePath, nil
	}
	// Otherwise treat as a file path
	return filepath.Abs(arg)
}

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
