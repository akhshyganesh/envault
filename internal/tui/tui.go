package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
)

// ── Views ─────────────────────────────────────────────────────────────────────

type view int

const (
	fileListView view = iota
	historyView
	contentView
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("57")).
			Padding(0, 1)
)

// ── Model ─────────────────────────────────────────────────────────────────────

type model struct {
	store    *store.Store
	files    []store.FileHistory
	current  view
	cursor   int
	history  *store.FileHistory
	hCursor  int
	viewport viewport.Model
	width    int
	height   int
	ready    bool
	content  string // rendered content for viewport
}

// Run launches the TUI.
func Run() error {
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

	m := model{
		store:   s,
		files:   files,
		current: fileListView,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return nil
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Set viewport dimensions for content view
		headerHeight := 4 // title + blank + header row + blank
		footerHeight := 2 // blank + status bar
		vpHeight := m.height - headerHeight - footerHeight
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.viewport = viewport.New(m.width-4, vpHeight)
		m.viewport.SetContent(m.content)
		return m, nil

	case tea.KeyMsg:
		switch m.current {
		case fileListView:
			return m.updateFileList(msg)
		case historyView:
			return m.updateHistory(msg)
		case contentView:
			return m.updateContent(msg)
		}
	}
	return m, nil
}

// ── File list key handling ────────────────────────────────────────────────────

func (m model) updateFileList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.files) - 1
	case "enter", "l", "right":
		// Open history for selected file
		f := m.files[m.cursor]
		h, err := m.store.GetHistory(f.FilePath)
		if err == nil && len(h.Snapshots) > 0 {
			m.history = h
			m.hCursor = len(h.Snapshots) - 1 // start at latest
			m.current = historyView
		}
	}
	return m, nil
}

// ── History key handling ──────────────────────────────────────────────────────

func (m model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "h", "left", "backspace":
		m.current = fileListView
		m.history = nil
	case "up", "k":
		if m.hCursor > 0 {
			m.hCursor--
		}
	case "down", "j":
		if m.hCursor < len(m.history.Snapshots)-1 {
			m.hCursor++
		}
	case "home", "g":
		m.hCursor = 0
	case "end", "G":
		m.hCursor = len(m.history.Snapshots) - 1
	case "enter", "l", "right":
		snap := m.history.Snapshots[m.hCursor]
		data, err := m.store.GetBlobContent(snap.ID)
		if err == nil {
			m.content = string(data)
			headerHeight := 4
			footerHeight := 2
			vpHeight := m.height - headerHeight - footerHeight
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport = viewport.New(m.width-4, vpHeight)
			m.viewport.SetContent(m.content)
			m.current = contentView
		}
	}
	return m, nil
}

// ── Content viewer key handling ───────────────────────────────────────────────

func (m model) updateContent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "h", "left", "backspace":
		m.current = historyView
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	switch m.current {
	case fileListView:
		return m.viewFileList()
	case historyView:
		return m.viewHistory()
	case contentView:
		return m.viewContent()
	}
	return ""
}

// ── File list view ────────────────────────────────────────────────────────────

func (m model) viewFileList() string {
	var b strings.Builder

	title := titleStyle.Render(" 🔒 envault — Tracked Files ")
	b.WriteString(title + "\n\n")

	// Column header
	header := headerStyle.Render(fmt.Sprintf("  %-4s %-60s %8s  %s", "#", "FILE", "VERSIONS", "LAST BACKUP"))
	b.WriteString(header + "\n")

	// Scroll window
	visibleRows := m.height - 6 // title + header + status bar + padding
	if visibleRows < 1 {
		visibleRows = 1
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(m.files) {
		end = len(m.files)
	}

	for i := start; i < end; i++ {
		f := m.files[i]
		last := f.Snapshots[len(f.Snapshots)-1]
		num := fmt.Sprintf("%d", i+1)
		path := format.ShortenPath(f.FilePath)
		if len(path) > 58 {
			path = "…" + path[len(path)-57:]
		}
		versions := fmt.Sprintf("%d", len(f.Snapshots))
		ago := format.TimeAgo(last.Timestamp)

		line := fmt.Sprintf("  %-4s %-60s %8s  %s", num, path, versions, ago)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	// Pad remaining space
	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-2 {
		b.WriteString("\n")
		rendered++
	}

	// Status bar
	status := fmt.Sprintf(" %d files | ↑↓/jk navigate | Enter select | q quit", len(m.files))
	scroll := ""
	if len(m.files) > visibleRows {
		scroll = fmt.Sprintf(" | %d/%d", m.cursor+1, len(m.files))
	}
	b.WriteString(statusBarStyle.Render(status + scroll))

	return b.String()
}

// ── History view ──────────────────────────────────────────────────────────────

func (m model) viewHistory() string {
	if m.history == nil {
		return ""
	}

	var b strings.Builder

	title := titleStyle.Render(fmt.Sprintf(" 📋 %s ", format.ShortenPath(m.history.FilePath)))
	b.WriteString(title + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d version(s)", len(m.history.Snapshots))) + "\n\n")

	// Column header
	header := headerStyle.Render(fmt.Sprintf("  %-4s %-14s %-18s %8s  %s", "#", "ID", "DATE", "SIZE", "COMMENT"))
	b.WriteString(header + "\n")

	visibleRows := m.height - 7
	if visibleRows < 1 {
		visibleRows = 1
	}

	start := 0
	if m.hCursor >= visibleRows {
		start = m.hCursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(m.history.Snapshots) {
		end = len(m.history.Snapshots)
	}

	for i := start; i < end; i++ {
		snap := m.history.Snapshots[i]
		num := fmt.Sprintf("%d", i+1)
		line := fmt.Sprintf("  %-4s %-14s %-18s %8s  %s",
			num,
			snap.ID[:12],
			snap.Timestamp.Local().Format("2006-01-02 15:04"),
			format.HumanSize(snap.Size),
			snap.Comment,
		)

		if i == m.hCursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-2 {
		b.WriteString("\n")
		rendered++
	}

	status := fmt.Sprintf(" %d versions | ↑↓/jk navigate | Enter view content | Esc back | q quit", len(m.history.Snapshots))
	scroll := ""
	if len(m.history.Snapshots) > visibleRows {
		scroll = fmt.Sprintf(" | %d/%d", m.hCursor+1, len(m.history.Snapshots))
	}
	b.WriteString(statusBarStyle.Render(status + scroll))

	return b.String()
}

// ── Content view ──────────────────────────────────────────────────────────────

func (m model) viewContent() string {
	if m.history == nil {
		return ""
	}

	snap := m.history.Snapshots[m.hCursor]

	var b strings.Builder

	title := titleStyle.Render(fmt.Sprintf(" 📄 %s — v%d (%s) ",
		format.ShortenPath(m.history.FilePath),
		m.hCursor+1,
		snap.ID[:12],
	))
	b.WriteString(title + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %s  •  %s",
		snap.Timestamp.Local().Format("2006-01-02 15:04:05"),
		format.HumanSize(snap.Size),
	)) + "\n\n")

	// Viewport content
	b.WriteString(borderStyle.Render(m.viewport.View()) + "\n")

	// Status bar
	pct := m.viewport.ScrollPercent()
	status := fmt.Sprintf(" ↑↓/jk scroll | Esc back | q quit | %.0f%%", pct*100)
	b.WriteString(statusBarStyle.Render(status))

	return b.String()
}

