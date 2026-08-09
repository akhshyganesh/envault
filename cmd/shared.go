package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
)

// mark is the tick that prefixes a successful result line. It is typographic,
// not an emoji, so it stays one cell wide in every terminal.
const mark = "✓"

// done prints a completed action.
func done(msg string, args ...any) {
	fmt.Printf(mark+" "+msg+"\n", args...)
}

// note prints an indented follow-up under a result line.
func note(msg string, args ...any) {
	fmt.Printf("  "+msg+"\n", args...)
}

// openStore is the first line of nearly every command.
func openStore() (*store.Store, error) {
	return store.NewStore()
}

// resolveFileArg accepts either the number shown by 'envault list' or a path,
// and returns an absolute path. Numbers are far easier to type, so they are
// tried first.
func resolveFileArg(arg string, s *store.Store) (string, error) {
	if idx, err := strconv.Atoi(arg); err == nil {
		files, err := s.ListTrackedFiles()
		if err != nil {
			return "", err
		}
		if idx < 1 || idx > len(files) {
			return "", fmt.Errorf("index %d out of range (1-%d) — run 'envault list' to see them", idx, len(files))
		}
		return files[idx-1].FilePath, nil
	}
	return filepath.Abs(arg)
}

// loadHistory resolves a file argument and returns its recorded versions,
// failing when nothing has ever been backed up for it.
func loadHistory(arg string, s *store.Store) (*store.FileHistory, error) {
	absPath, err := resolveFileArg(arg, s)
	if err != nil {
		return nil, err
	}
	h, err := s.History(absPath)
	if err != nil {
		return nil, err
	}
	if len(h.Snapshots) == 0 {
		return nil, fmt.Errorf("no backups found for %s", format.ShortenPath(absPath))
	}
	return h, nil
}

// pickSnapshot selects a 1-based version, defaulting to the newest.
func pickSnapshot(snaps []store.Snapshot, version int) (*store.Snapshot, error) {
	if version <= 0 {
		return &snaps[len(snaps)-1], nil
	}
	if version > len(snaps) {
		return nil, fmt.Errorf("version %d not found (this file has %d)", version, len(snaps))
	}
	return &snaps[version-1], nil
}

// shortID trims a SHA-256 to the 12 characters that identify it in practice.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// table returns a tabwriter over stdout, so every listing lines up the same way.
func table() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// printVersions renders a file's history as a numbered table.
func printVersions(w *tabwriter.Writer, snaps []store.Snapshot) {
	fmt.Fprintln(w, "#\tID\tDATE\tSIZE\tCOMMENT")
	for i, s := range snaps {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			i+1,
			shortID(s.ID),
			s.Timestamp.Local().Format("2006-01-02 15:04"),
			format.HumanSize(s.Size),
			s.Comment)
	}
}

// latestReleaseTag asks GitHub for the newest published tag. Redirects are not
// followed so a moved endpoint surfaces as an error rather than as HTML.
func latestReleaseTag() (string, error) {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
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
