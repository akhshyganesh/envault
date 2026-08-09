package tui

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
)

// Every message below is the result of one background command. They exist so
// that slow work — scanning a home directory, unzipping an archive — never
// blocks the render loop.

type (
	clipboardClearedMsg struct{}
	statusClearedMsg    struct{}
)

type filesLoadedMsg struct {
	files []store.FileHistory
	err   error
}

type scanDoneMsg struct {
	found, backed, skipped int
	err                    error
}

type restoreDoneMsg struct {
	path string
	err  error
}

type exportDoneMsg struct {
	res *transfer.ExportResult
	err error
}

type importDoneMsg struct {
	count int
	err   error
}

// vaultExistsMsg asks the user to confirm before an import overwrites a vault.
type vaultExistsMsg struct{ zipPath string }

type watchDoneMsg struct {
	path string
	err  error
}

type daemonDoneMsg struct {
	running bool
	pid     int
	err     error
}

func loadFilesCmd(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		files, err := s.ListTrackedFiles()
		return filesLoadedMsg{files: files, err: err}
	}
}

func scanCmd(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return scanDoneMsg{err: err}
		}
		res, err := scanner.ScanDirectories(cfg.WatchDirs, s)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		return scanDoneMsg{found: len(res.Found), backed: res.Backed, skipped: res.Skipped}
	}
}

func restoreCmd(s *store.Store, path, snapshotID string) tea.Cmd {
	return func() tea.Msg {
		return restoreDoneMsg{path: path, err: s.Restore(path, snapshotID)}
	}
}

func restoreToCmd(s *store.Store, rawPath, snapshotID string) tea.Cmd {
	return func() tea.Msg {
		abs, err := format.ExpandPath(rawPath)
		if err != nil {
			return restoreDoneMsg{err: err}
		}
		return restoreDoneMsg{path: abs, err: s.Restore(abs, snapshotID)}
	}
}

func exportCmd(path string) tea.Cmd {
	return func() tea.Msg {
		res, err := transfer.Export(path)
		return exportDoneMsg{res: res, err: err}
	}
}

// importCmd reports a pre-existing vault separately, so the caller can ask for
// confirmation instead of turning a recoverable situation into an error.
func importCmd(rawPath string, force bool) tea.Cmd {
	return func() tea.Msg {
		abs, err := format.ExpandPath(rawPath)
		if err != nil {
			return importDoneMsg{err: err}
		}
		count, err := transfer.Import(abs, force)
		if !force && errorIsVaultExists(err) {
			return vaultExistsMsg{zipPath: abs}
		}
		return importDoneMsg{count: count, err: err}
	}
}

func watchCmd(rawPath string) tea.Cmd {
	return func() tea.Msg {
		abs, err := format.ExpandPath(rawPath)
		if err != nil {
			return watchDoneMsg{err: err}
		}
		if _, err := config.AddWatchDir(abs); err != nil {
			return watchDoneMsg{err: err}
		}
		return watchDoneMsg{path: abs}
	}
}

// toggleDaemonCmd starts the daemon as a detached child in its own process
// group, so quitting the TUI does not take the daemon down with it.
func toggleDaemonCmd(running bool) tea.Cmd {
	return func() tea.Msg {
		if running {
			if err := daemon.Stop(); err != nil {
				alive, pid := daemon.IsRunning()
				return daemonDoneMsg{running: alive, pid: pid, err: err}
			}
			return daemonDoneMsg{running: false}
		}

		exe, err := os.Executable()
		if err != nil {
			return daemonDoneMsg{err: err}
		}
		c := exec.Command(exe, "start")
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := c.Start(); err != nil {
			return daemonDoneMsg{err: err}
		}
		// Give the child a moment to claim the PID file so the reply is true.
		time.Sleep(400 * time.Millisecond)
		alive, pid := daemon.IsRunning()
		return daemonDoneMsg{running: alive, pid: pid}
	}
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return statusClearedMsg{} })
}

func clearClipboardNoticeAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clipboardClearedMsg{} })
}
