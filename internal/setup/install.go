package setup

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
)

// progressLine is one line of step 3's output.
type progressLine struct {
	text string
	warn bool
}

// progressMsg carries a line from the install goroutine into the render loop.
// A nil line with done set means the run finished cleanly.
type progressMsg struct {
	line *progressLine
	done bool
	err  error
}

// waitForProgress blocks in a tea.Cmd until the installer sends its next line.
func waitForProgress(ch chan progressMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// runInstall does the actual work: save the config, take a first backup, and
// register the startup service. It streams progress rather than returning at
// the end, because scanning a home directory takes long enough to need it.
//
// A failed service install is a warning, not a failure: the vault is already
// configured and scanning by hand still works.
func runInstall(ch chan progressMsg, dirs []string, intervalSecs int) {
	defer close(ch)

	say := func(text string) { ch <- progressMsg{line: &progressLine{text: text}} }
	warn := func(text string) { ch <- progressMsg{line: &progressLine{text: text, warn: true}} }

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.WatchDirs = dirs
	cfg.ScanIntervalSecs = intervalSecs
	if err := cfg.Save(); err != nil {
		ch <- progressMsg{err: fmt.Errorf("saving config: %w", err), done: true}
		return
	}
	say(fmt.Sprintf("Config saved — watching %d %s", len(dirs), plural(len(dirs), "directory", "directories")))

	s, err := store.NewStore()
	if err != nil {
		ch <- progressMsg{err: fmt.Errorf("opening vault: %w", err), done: true}
		return
	}
	for _, dir := range dirs {
		res, err := scanner.ScanDirectory(dir, s)
		if err != nil {
			warn(fmt.Sprintf("Could not scan %s: %v", format.ShortenPath(dir), err))
			continue
		}
		say(fmt.Sprintf("%-38s  %d %s, %d new",
			format.ShortenPath(dir), len(res.Found), plural(len(res.Found), "file", "files"), res.Backed))
	}

	if note, err := daemon.Install(); err != nil {
		warn(fmt.Sprintf("Startup service: %v", err))
	} else {
		say(note)
	}

	ch <- progressMsg{done: true}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
