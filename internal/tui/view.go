package tui

import (
	"fmt"
	"strings"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/ui/theme"
)

// Column widths for the two list views. Kept here so the header and the rows
// can never drift apart.
const (
	colNum  = 4
	colPath = 58
	colID   = 14
	colDate = 18
	colSize = 8
)

// chromeRows is how many lines a list view spends on things that are not rows:
// breadcrumb, rule, column header, status bar, and a blank.
const chromeRows = 6

func (m model) View() string {
	if !m.ready {
		return "Loading…"
	}
	switch m.current {
	case fileListView:
		return m.viewFileList()
	case historyView:
		return m.viewHistory()
	case contentView:
		return m.viewContent()
	case promptView:
		return m.viewPrompt()
	}
	return ""
}

func (m model) viewFileList() string {
	var b strings.Builder
	b.WriteString(theme.Header(m.width, "envault", "tracked files"))
	b.WriteString(theme.ColumnRow.Render(fmt.Sprintf("  %-*s %-*s %*s  %s",
		colNum, "#", colPath, "FILE", colSize, "VERSIONS", "LAST BACKUP")) + "\n")

	rows := m.visibleRows()
	start, end := window(m.cursor, len(m.files), rows)
	for i := start; i < end; i++ {
		f := m.files[i]
		versions := len(f.Snapshots)
		last := "—"
		if latest := f.Latest(); latest != nil {
			last = format.TimeAgo(latest.Timestamp)
		}
		line := fmt.Sprintf("%-*d %-*s %*d  %s",
			colNum, i+1,
			colPath, theme.Truncate(format.ShortenPath(f.FilePath), colPath),
			colSize, versions, last)
		b.WriteString(m.renderRow(line, i == m.cursor))
	}

	m.pad(&b)
	b.WriteString(m.statusBar(
		fmt.Sprintf("%d files%s", len(m.files), scrollHint(m.cursor, len(m.files), rows)),
		theme.Hints(
			theme.Hint{Key: "s", Action: "scan"},
			theme.Hint{Key: "d", Action: "daemon"},
			theme.Hint{Key: "w", Action: "watch"},
			theme.Hint{Key: "e", Action: "export"},
			theme.Hint{Key: "i", Action: "import"},
			theme.Hint{Key: theme.GlyphEnter, Action: "open"},
			theme.Hint{Key: "q", Action: "quit"},
		),
	))
	return b.String()
}

func (m model) viewHistory() string {
	if m.history == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(theme.Header(m.width, "envault", format.ShortenPath(m.history.FilePath)))
	b.WriteString(theme.ColumnRow.Render(fmt.Sprintf("  %-*s %-*s %-*s %*s  %s",
		colNum, "#", colID, "ID", colDate, "DATE", colSize, "SIZE", "COMMENT")) + "\n")

	snaps := m.history.Snapshots
	rows := m.visibleRows()
	start, end := window(m.hCursor, len(snaps), rows)
	for i := start; i < end; i++ {
		s := snaps[i]
		line := fmt.Sprintf("%-*d %-*s %-*s %*s  %s",
			colNum, i+1,
			colID, shortID(s.ID),
			colDate, s.Timestamp.Local().Format("2006-01-02 15:04"),
			colSize, format.HumanSize(s.Size),
			s.Comment)
		b.WriteString(m.renderRow(line, i == m.hCursor))
	}

	m.pad(&b)
	b.WriteString(m.statusBar(
		fmt.Sprintf("%d versions%s", len(snaps), scrollHint(m.hCursor, len(snaps), rows)),
		theme.Hints(
			theme.Hint{Key: "r", Action: "restore"},
			theme.Hint{Key: "R", Action: "restore to"},
			theme.Hint{Key: theme.GlyphEnter, Action: "view"},
			theme.Hint{Key: "esc", Action: "back"},
			theme.Hint{Key: "q", Action: "quit"},
		),
	))
	return b.String()
}

func (m model) viewContent() string {
	snap := m.selectedSnapshot()
	if snap == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(theme.Header(m.width, "envault",
		format.ShortenPath(m.history.FilePath),
		fmt.Sprintf("v%d", m.hCursor+1)))
	b.WriteString(theme.Dim.Render(fmt.Sprintf("  %s%s%s%s%s",
		shortID(snap.ID), theme.Separator(),
		snap.Timestamp.Local().Format("2006-01-02 15:04:05"), theme.Separator(),
		format.HumanSize(snap.Size))) + "\n")
	b.WriteString(theme.Box.Render(m.viewport.View()) + "\n")

	hints := theme.Hints(
		theme.Hint{Key: "c", Action: "copy"},
		theme.Hint{Key: "r", Action: "restore"},
		theme.Hint{Key: "R", Action: "restore to"},
		theme.Hint{Key: "↑↓", Action: "scroll"},
		theme.Hint{Key: "esc", Action: "back"},
		theme.Hint{Key: "q", Action: "quit"},
	)
	if m.copiedNotice != "" {
		hints = m.copiedNotice
	}
	b.WriteString(m.statusBar(fmt.Sprintf("%.0f%%", m.viewport.ScrollPercent()*100), hints))
	return b.String()
}

func (m model) viewPrompt() string {
	var b strings.Builder
	b.WriteString(theme.Header(m.width, "envault"))
	b.WriteString("\n")
	b.WriteString(theme.Brand.Render("  "+m.promptLabel) + "\n\n")
	b.WriteString("  " + m.input.View() + "\n\n")

	if m.prompt == promptConfirmOverwrite {
		b.WriteString(theme.Bad.Render("  "+theme.GlyphWarn+"  This replaces every backup in the vault.") + "\n")
	}

	m.pad(&b)
	b.WriteString(m.statusBar("", theme.Hints(
		theme.Hint{Key: theme.GlyphEnter, Action: "confirm"},
		theme.Hint{Key: "esc", Action: "cancel"},
	)))
	return b.String()
}

// ── Rendering helpers ────────────────────────────────────────────────────────

// renderRow draws one list row, marking the selected one with an accent gutter
// bar rather than a full-width highlight.
func (m model) renderRow(line string, selected bool) string {
	if selected {
		return theme.Gutter.Render(theme.GlyphGutter+" ") + theme.Selected.Render(line) + "\n"
	}
	return "  " + theme.Normal.Render(line) + "\n"
}

// statusBar pins the daemon indicator to the right and shows either the hint
// keys or, while one is pending, a transient status message.
func (m model) statusBar(count, hints string) string {
	daemon := theme.Dim.Render(theme.GlyphOff + " daemon")
	if m.daemonRunning {
		daemon = theme.Good.Render(fmt.Sprintf("%s pid %d", theme.GlyphOn, m.daemonPID))
	}

	main := hints
	if m.statusMsg != "" {
		main = m.statusMsg
	}

	content := " " + main + "  " + daemon
	if count != "" {
		content = theme.Key.Render(count) + theme.Separator() + main + "  " + daemon
	}
	return theme.StatusBar.Render(content)
}

// pad fills the gap between the rendered rows and the status bar so the bar
// stays welded to the bottom of the window.
func (m model) pad(b *strings.Builder) {
	for lines := strings.Count(b.String(), "\n"); lines < m.height-2; lines++ {
		b.WriteString("\n")
	}
}

func (m model) visibleRows() int {
	return max(m.height-chromeRows, 1)
}

// window returns the slice of rows to draw so that the cursor stays on screen.
func window(cursor, count, rows int) (start, end int) {
	if cursor >= rows {
		start = cursor - rows + 1
	}
	return start, min(start+rows, count)
}

// scrollHint shows "3/40" only when the list is actually taller than the window.
func scrollHint(cursor, count, rows int) string {
	if count <= rows {
		return ""
	}
	return fmt.Sprintf(" %d/%d", cursor+1, count)
}

// shortID trims a SHA-256 to the 12 characters that identify it in practice.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
