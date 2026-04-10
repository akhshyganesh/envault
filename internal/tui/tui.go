package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type clipboardClearedMsg  struct{}
type statusClearedMsg     struct{}
type reloadFilesMsg       struct{ files []store.FileHistory; err error }
type scanDoneMsg          struct{ found, backed, skipped int; err error }
type restoreDoneMsg       struct{ path string; err error }
type exportDoneMsg        struct{ path string; size int64; count int; err error }
type importDoneMsg        struct{ count int; err error }
type importVaultExistsMsg struct{ zipPath string }
type watchDoneMsg         struct{ path string; err error }
type daemonToggleDoneMsg  struct{ running bool; pid int; err error }

// ── Views ─────────────────────────────────────────────────────────────────────

type view int

const (
	fileListView view = iota
	historyView
	contentView
	inputView
)

// ── Input actions ─────────────────────────────────────────────────────────────

type inputAction int

const (
	inputWatchDir        inputAction = iota
	inputExport
	inputImport
	inputRestoreCustom
	inputConfirmOverwrite
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

	daemonOnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	daemonOffStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
)

// ── Model ─────────────────────────────────────────────────────────────────────

type model struct {
	store   *store.Store
	files   []store.FileHistory
	current view
	cursor  int
	history *store.FileHistory
	hCursor int

	viewport viewport.Model
	width    int
	height   int
	ready    bool
	content  string

	copiedNotice string

	// Daemon state
	daemonRunning bool
	daemonPID     int

	// Transient status bar message
	statusMsg string

	// Input prompt state
	inputAction inputAction
	inputPrompt string
	textInput   textinput.Model
	prevView    view

	// Context for restore-to-custom-path
	pendingRestoreSnap string

	// Context for import overwrite confirmation
	pendingImportPath string
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

	daemonRunning, daemonPID := daemon.IsRunning()
	m := model{
		store:         s,
		files:         files,
		current:       fileListView,
		daemonRunning: daemonRunning,
		daemonPID:     daemonPID,
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
		case inputView:
			return m.updateInput(msg)
		}

	case clipboardClearedMsg:
		m.copiedNotice = ""
		return m, nil

	case statusClearedMsg:
		m.statusMsg = ""
		return m, nil

	case reloadFilesMsg:
		if msg.err == nil {
			m.files = msg.files
			if len(m.files) > 0 && m.cursor >= len(m.files) {
				m.cursor = len(m.files) - 1
			}
		}
		return m, nil

	case scanDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Scan failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("✓ Scan: %d found, %d new, %d unchanged", msg.found, msg.backed, msg.skipped)
		}
		return m, tea.Batch(reloadFilesCmd(m.store), clearStatusAfter(4*time.Second))

	case restoreDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Restore failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("✓ Restored → %s", format.ShortenPath(msg.path))
		}
		return m, clearStatusAfter(3 * time.Second)

	case exportDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Export failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("✓ Exported %d files → %s (%s)", msg.count, format.ShortenPath(msg.path), format.HumanSize(msg.size))
		}
		return m, clearStatusAfter(5 * time.Second)

	case importDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Import failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("✓ Imported %d files", msg.count)
		}
		return m, tea.Batch(reloadFilesCmd(m.store), clearStatusAfter(4*time.Second))

	case importVaultExistsMsg:
		m.pendingImportPath = msg.zipPath
		return m.openInput(inputConfirmOverwrite, "Vault exists! Type YES to overwrite all data:", "", fileListView)

	case watchDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("✓ Now watching %s", format.ShortenPath(msg.path))
		}
		return m, clearStatusAfter(3 * time.Second)

	case daemonToggleDoneMsg:
		m.daemonRunning = msg.running
		m.daemonPID = msg.pid
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Daemon: %v", msg.err)
		} else if msg.running {
			m.statusMsg = fmt.Sprintf("✓ Daemon started (PID %d)", msg.pid)
		} else {
			m.statusMsg = "✓ Daemon stopped"
		}
		return m, clearStatusAfter(3 * time.Second)
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
	case "s":
		s := m.store
		return m, func() tea.Msg {
			cfg, err := config.Load()
			if err != nil {
				return scanDoneMsg{err: err}
			}
			result, err := scanner.ScanDirectories(cfg.WatchDirs, s)
			if err != nil {
				return scanDoneMsg{err: err}
			}
			return scanDoneMsg{found: len(result.Found), backed: result.Backed, skipped: result.Skipped}
		}
	case "d":
		running := m.daemonRunning
		return m, func() tea.Msg {
			if running {
				if err := daemon.Stop(); err != nil {
					r, pid := daemon.IsRunning()
					return daemonToggleDoneMsg{running: r, pid: pid, err: err}
				}
				return daemonToggleDoneMsg{running: false}
			}
			exe, err := os.Executable()
			if err != nil {
				return daemonToggleDoneMsg{err: err}
			}
			cmd := exec.Command(exe, "start")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := cmd.Start(); err != nil {
				return daemonToggleDoneMsg{err: err}
			}
			time.Sleep(400 * time.Millisecond)
			r, pid := daemon.IsRunning()
			return daemonToggleDoneMsg{running: r, pid: pid}
		}
	case "w":
		return m.openInput(inputWatchDir, "Add watch directory:", "", fileListView)
	case "e":
		defaultName := fmt.Sprintf("envault-backup-%s.zip", time.Now().Format("20060102-150405"))
		return m.openInput(inputExport, "Export to zip file:", defaultName, fileListView)
	case "i":
		return m.openInput(inputImport, "Import from zip file:", "", fileListView)
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
	case "r":
		if m.history != nil {
			snap := m.history.Snapshots[m.hCursor]
			filePath := m.history.FilePath
			s := m.store
			return m, func() tea.Msg {
				err := s.RestoreSnapshot(filePath, snap.ID)
				return restoreDoneMsg{path: filePath, err: err}
			}
		}
	case "R":
		if m.history != nil {
			snap := m.history.Snapshots[m.hCursor]
			m.pendingRestoreSnap = snap.ID
			return m.openInput(inputRestoreCustom, "Restore to path:", m.history.FilePath, historyView)
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
	case "c":
		if err := clipboard.WriteAll(m.content); err == nil {
			m.copiedNotice = "✓ Copied to clipboard!"
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return clipboardClearedMsg{}
			})
		}
	case "r":
		if m.history != nil {
			snap := m.history.Snapshots[m.hCursor]
			filePath := m.history.FilePath
			s := m.store
			return m, func() tea.Msg {
				err := s.RestoreSnapshot(filePath, snap.ID)
				return restoreDoneMsg{path: filePath, err: err}
			}
		}
	case "R":
		if m.history != nil {
			snap := m.history.Snapshots[m.hCursor]
			m.pendingRestoreSnap = snap.ID
			return m.openInput(inputRestoreCustom, "Restore to path:", m.history.FilePath, contentView)
		}
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── Input prompt key handling ─────────────────────────────────────────────────

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.current = m.prevView
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.textInput.Value())
		if val == "" {
			m.current = m.prevView
			return m, nil
		}
		m.current = m.prevView
		action := m.inputAction
		switch action {
		case inputWatchDir:
			return m, func() tea.Msg {
				abs, err := format.ExpandPath(val)
				if err != nil {
					return watchDoneMsg{err: err}
				}
				_, err = config.AddWatchDir(abs)
				if err != nil {
					return watchDoneMsg{err: err}
				}
				return watchDoneMsg{path: abs}
			}
		case inputExport:
			return m, func() tea.Msg {
				path, count, size, err := transfer.Export(val)
				return exportDoneMsg{path: path, count: count, size: size, err: err}
			}
		case inputImport:
			return m, func() tea.Msg {
				abs, err := format.ExpandPath(val)
				if err != nil {
					return importDoneMsg{err: err}
				}
				count, err := transfer.Import(abs, false)
				if err != nil && errors.Is(err, transfer.ErrVaultExists) {
					return importVaultExistsMsg{zipPath: abs}
				}
				return importDoneMsg{count: count, err: err}
			}
		case inputRestoreCustom:
			restoreSnap := m.pendingRestoreSnap
			s := m.store
			return m, func() tea.Msg {
				abs, err := format.ExpandPath(val)
				if err != nil {
					return restoreDoneMsg{err: err}
				}
				err = s.RestoreSnapshot(abs, restoreSnap)
				return restoreDoneMsg{path: abs, err: err}
			}
		case inputConfirmOverwrite:
			if strings.ToUpper(val) == "YES" {
				zipPath := m.pendingImportPath
				return m, func() tea.Msg {
					count, err := transfer.Import(zipPath, true)
					return importDoneMsg{count: count, err: err}
				}
			}
			m.statusMsg = "Import cancelled"
			return m, clearStatusAfter(2 * time.Second)
		}
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// openInput configures and activates the text input view.
func (m model) openInput(action inputAction, prompt, defaultVal string, returnTo view) (model, tea.Cmd) {
	ti := textinput.New()
	ti.SetValue(defaultVal)
	if defaultVal == "" {
		ti.Placeholder = "enter path…"
	}
	width := m.width - 20
	if width < 40 {
		width = 40
	}
	ti.Width = width
	ti.CharLimit = 512
	ti.Focus()
	m.textInput = ti
	m.inputAction = action
	m.inputPrompt = prompt
	m.prevView = returnTo
	m.current = inputView
	return m, textinput.Blink
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
	case inputView:
		return m.viewInput()
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
	scrollPart := ""
	if len(m.files) > visibleRows {
		scrollPart = fmt.Sprintf(" %d/%d", m.cursor+1, len(m.files))
	}
	b.WriteString(m.buildStatusBar(
		fmt.Sprintf(" %d files%s", len(m.files), scrollPart),
		"s scan · d daemon · w watch · e export · i import · ↑↓ navigate · Enter open · q quit",
	))

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

	scrollPart := ""
	if len(m.history.Snapshots) > visibleRows {
		scrollPart = fmt.Sprintf(" %d/%d", m.hCursor+1, len(m.history.Snapshots))
	}
	b.WriteString(m.buildStatusBar(
		fmt.Sprintf(" %d versions%s", len(m.history.Snapshots), scrollPart),
		"r restore · R restore to… · ↑↓ navigate · Enter view · Esc back · q quit",
	))

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
	var hints string
	if m.copiedNotice != "" {
		hints = m.copiedNotice
	} else {
		hints = fmt.Sprintf("c copy · r restore · R restore to… · ↑↓ scroll · Esc back · q quit · %.0f%%", pct*100)
	}
	b.WriteString(m.buildStatusBar("", hints))

	return b.String()
}

// ── Input view ────────────────────────────────────────────────────────────────

func (m model) viewInput() string {
	var b strings.Builder

	title := titleStyle.Render(" 🔒 envault ")
	b.WriteString(title + "\n\n")

	b.WriteString(headerStyle.Render("  "+m.inputPrompt) + "\n\n")
	b.WriteString("  " + m.textInput.View() + "\n\n")

	if m.inputAction == inputConfirmOverwrite {
		b.WriteString(dimStyle.Render("  ⚠  This will overwrite all existing vault data.") + "\n")
	}
	b.WriteString(dimStyle.Render("  Enter to confirm · Esc to cancel") + "\n")

	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-2 {
		b.WriteString("\n")
		rendered++
	}
	b.WriteString(m.buildStatusBar("", "Enter confirm · Esc cancel · ctrl+c quit"))

	return b.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// buildStatusBar renders the status bar. countPart is the leftmost section (e.g. "5 files"),
// hints is the action hint text (replaced by statusMsg when one is active).
// The daemon indicator is always shown on the right.
func (m model) buildStatusBar(countPart, hints string) string {
	var daemonStr string
	if m.daemonRunning {
		daemonStr = "  " + daemonOnStyle.Render(fmt.Sprintf("● pid %d", m.daemonPID))
	} else {
		daemonStr = "  " + daemonOffStyle.Render("○ daemon")
	}

	mainText := hints
	if m.statusMsg != "" {
		mainText = m.statusMsg
	}

	var content string
	if countPart != "" {
		content = countPart + " · " + mainText + daemonStr
	} else {
		content = " " + mainText + daemonStr
	}
	return statusBarStyle.Render(content)
}

// ── Tea commands ──────────────────────────────────────────────────────────────

func reloadFilesCmd(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		files, err := s.ListTrackedFiles()
		return reloadFilesMsg{files: files, err: err}
	}
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusClearedMsg{}
	})
}

