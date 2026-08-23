package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/transfer"
	"github.com/akhshyganesh/envault/internal/ui/theme"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
		m.viewport = m.makeViewport(m.content)
		return m, nil

	case tea.KeyMsg:
		switch m.current {
		case fileListView:
			return m.onFileListKey(msg)
		case historyView:
			return m.onHistoryKey(msg)
		case contentView:
			return m.onContentKey(msg)
		case promptView:
			return m.onPromptKey(msg)
		}
		return m, nil

	default:
		return m.onResult(msg)
	}
}

// onResult folds a finished background command back into the model. Each arm
// sets a status line and, where the vault changed, reloads the file list.
func (m model) onResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clipboardClearedMsg:
		m.copiedNotice = ""

	case statusClearedMsg:
		m.statusMsg = ""

	case filesLoadedMsg:
		if msg.err == nil {
			m.files = msg.files
			if !m.showArchived && m.cursor >= len(m.files) {
				m.cursor = max(len(m.files)-1, 0)
			}
		}

	case archivedLoadedMsg:
		if msg.err == nil {
			m.archived = msg.archived
			if m.showArchived && m.cursor >= len(m.archived) {
				m.cursor = max(len(m.archived)-1, 0)
			}
		}

	case archiveDoneMsg:
		if msg.err != nil {
			return m.fail("Archive", msg.err, 4*time.Second)
		}
		m.statusMsg = m.ok("Archived " + format.ShortenPath(msg.path))
		return m, tea.Batch(loadFilesCmd(m.store), loadShelfCmd(m.store), clearStatusAfter(4*time.Second))

	case scanDoneMsg:
		if msg.err != nil {
			return m.fail("Scan failed", msg.err, 4*time.Second)
		}
		m.statusMsg = m.ok(fmt.Sprintf("Scan: %d found, %d new, %d unchanged", msg.found, msg.backed, msg.skipped))
		return m, tea.Batch(loadFilesCmd(m.store), clearStatusAfter(4*time.Second))

	case restoreDoneMsg:
		if msg.err != nil {
			return m.fail("Restore failed", msg.err, 4*time.Second)
		}
		m.statusMsg = m.ok("Restored " + format.ShortenPath(msg.path))
		return m, clearStatusAfter(3 * time.Second)

	case exportDoneMsg:
		if msg.err != nil {
			return m.fail("Export failed", msg.err, 5*time.Second)
		}
		m.statusMsg = m.ok(fmt.Sprintf("Exported %d files to %s (%s)",
			msg.res.Files, format.ShortenPath(msg.res.Path), format.HumanSize(msg.res.Size)))
		return m, clearStatusAfter(5 * time.Second)

	case importDoneMsg:
		if msg.err != nil {
			return m.fail("Import failed", msg.err, 5*time.Second)
		}
		m.statusMsg = m.ok(fmt.Sprintf("Imported %d files", msg.count))
		return m, tea.Batch(loadFilesCmd(m.store), clearStatusAfter(4*time.Second))

	case vaultExistsMsg:
		m.pendingImportPath = msg.zipPath
		return m.openPrompt(promptConfirmOverwrite, "Type YES to replace the vault:", "", fileListView)

	case watchDoneMsg:
		if msg.err != nil {
			return m.fail("", msg.err, 4*time.Second)
		}
		m.statusMsg = m.ok("Now watching " + format.ShortenPath(msg.path))
		return m, clearStatusAfter(3 * time.Second)

	case daemonDoneMsg:
		m.daemonRunning, m.daemonPID = msg.running, msg.pid
		switch {
		case msg.err != nil:
			return m.fail("Daemon", msg.err, 4*time.Second)
		case msg.running:
			m.statusMsg = m.ok(fmt.Sprintf("Daemon started (PID %d)", msg.pid))
		default:
			m.statusMsg = m.ok("Daemon stopped")
		}
		return m, clearStatusAfter(3 * time.Second)
	}
	return m, nil
}

func (m model) ok(text string) string {
	return theme.Good.Render(theme.GlyphOK) + " " + text
}

func (m model) fail(prefix string, err error, d time.Duration) (tea.Model, tea.Cmd) {
	text := err.Error()
	if prefix != "" {
		text = prefix + ": " + text
	}
	m.statusMsg = theme.Bad.Render(theme.GlyphBad) + " " + text
	return m, clearStatusAfter(d)
}

// ── Key handling ─────────────────────────────────────────────────────────────

// moveCursor applies the shared navigation keys and reports whether it handled
// the key, so each view only spells out what is unique to it.
func moveCursor(cursor *int, count int, key string) bool {
	switch key {
	case "up", "k":
		if *cursor > 0 {
			*cursor--
		}
	case "down", "j":
		if *cursor < count-1 {
			*cursor++
		}
	case "home", "g":
		*cursor = 0
	case "end", "G":
		*cursor = max(count-1, 0)
	default:
		return false
	}
	return true
}

func (m model) onFileListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	list := m.listing()
	if moveCursor(&m.cursor, len(list), key) {
		return m, nil
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "A":
		m.showArchived = !m.showArchived
		if m.cursor >= len(m.listing()) {
			m.cursor = max(len(m.listing())-1, 0)
		}
		return m, nil

	case "a":
		if m.showArchived || len(list) == 0 {
			return m, nil
		}
		return m, archiveCmd(m.store, list[m.cursor].FilePath)

	case "u":
		if !m.showArchived || len(list) == 0 {
			return m, nil
		}
		return m, unarchiveCmd(m.store, list[m.cursor].FilePath)

	case "enter", "l", "right":
		if len(list) == 0 {
			return m, nil
		}
		h, err := m.store.History(list[m.cursor].FilePath)
		if err != nil || len(h.Snapshots) == 0 {
			return m, nil
		}
		m.history = h
		m.hCursor = len(h.Snapshots) - 1 // open on the newest version
		m.current = historyView

	case "s":
		return m, scanCmd(m.store)

	case "d":
		return m, toggleDaemonCmd(m.daemonRunning)

	case "w":
		return m.openPrompt(promptWatchDir, "Watch which directory?", "", fileListView)

	case "e":
		return m.openPrompt(promptExport, "Export to which file?", transfer.DefaultExportName(), fileListView)

	case "i":
		return m.openPrompt(promptImport, "Import which zip?", "", fileListView)
	}
	return m, nil
}

func (m model) onHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.history == nil {
		m.current = fileListView
		return m, nil
	}

	key := msg.String()
	if moveCursor(&m.hCursor, len(m.history.Snapshots), key) {
		return m, nil
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc", "h", "left", "backspace":
		m.current = fileListView
		m.history = nil

	case "enter", "l", "right":
		snap := m.selectedSnapshot()
		if snap == nil {
			return m, nil
		}
		data, err := m.store.Content(snap.ID)
		if err != nil {
			return m.fail("Cannot read version", err, 4*time.Second)
		}
		m.content = string(data)
		m.viewport = m.makeViewport(m.content)
		m.current = contentView

	case "r":
		return m.restoreSelected()

	case "R":
		return m.promptRestoreTo(historyView)
	}
	return m, nil
}

func (m model) onContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc", "h", "left", "backspace":
		m.current = historyView

	case "c":
		if err := clipboard.WriteAll(m.content); err != nil {
			return m.fail("Copy failed", err, 4*time.Second)
		}
		m.copiedNotice = theme.Good.Render(theme.GlyphOK) + " Copied to clipboard"
		return m, clearClipboardNoticeAfter(2 * time.Second)

	case "r":
		return m.restoreSelected()

	case "R":
		return m.promptRestoreTo(contentView)

	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) restoreSelected() (tea.Model, tea.Cmd) {
	snap := m.selectedSnapshot()
	if snap == nil {
		return m, nil
	}
	return m, restoreCmd(m.store, m.history.FilePath, snap.ID)
}

func (m model) promptRestoreTo(returnTo view) (tea.Model, tea.Cmd) {
	snap := m.selectedSnapshot()
	if snap == nil {
		return m, nil
	}
	m.pendingSnapshotID = snap.ID
	return m.openPrompt(promptRestoreTo, "Restore to which path?", m.history.FilePath, returnTo)
}

func (m model) onPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.current = m.returnTo
		return m, nil

	case "enter":
		value := strings.TrimSpace(m.input.Value())
		m.current = m.returnTo
		if value == "" {
			return m, nil
		}
		return m.runPrompt(value)

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m model) runPrompt(value string) (tea.Model, tea.Cmd) {
	switch m.prompt {
	case promptWatchDir:
		return m, watchCmd(value)

	case promptExport:
		return m, exportCmd(value)

	case promptImport:
		return m, importCmd(value, false)

	case promptRestoreTo:
		return m, restoreToCmd(m.store, value, m.pendingSnapshotID)

	case promptConfirmOverwrite:
		// Anything but an explicit YES leaves the existing vault alone.
		if !strings.EqualFold(value, "yes") {
			m.statusMsg = "Import cancelled"
			return m, clearStatusAfter(2 * time.Second)
		}
		return m, importCmd(m.pendingImportPath, true)
	}
	return m, nil
}
