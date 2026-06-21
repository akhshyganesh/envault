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

type clipboardClearedMsg struct{}
type statusClearedMsg struct{}
type reloadFilesMsg struct {
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
	path  string
	size  int64
	count int
	err   error
}
type importDoneMsg struct {
	count int
	err   error
}
type importVaultExistsMsg struct{ zipPath string }
type watchDoneMsg struct {
	path string
	err  error
}
type daemonToggleDoneMsg struct {
	running bool
	pid     int
	err     error
}

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
	inputWatchDir inputAction = iota
	inputExport
	inputImport
	inputRestoreCustom
	inputConfirmOverwrite
)

// ── Palette ───────────────────────────────────────────────────────────────────
// A warm amber accent on neutral grays — the envault identity, tuned to read like
// a modern command-line interface rather than the default BubbleTea theme.
const (
	colAccent   = "180" // warm amber — brand and primary actions
	colAccentHi = "215" // brighter amber for the selected row
	colText     = "252" // primary foreground
	colMuted    = "245" // secondary text
	colFaint    = "240" // rules and tertiary detail
	colSuccess  = "114" // soft green
	colWarn     = "215"
	colBarBg    = "236" // status bar background
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	brandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colAccent)).
			Bold(true)

	crumbStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colText))

	ruleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFaint))

	gutterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colAccent)).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colAccentHi)).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colText))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colMuted))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colMuted)).
			Background(lipgloss.Color(colBarBg)).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colMuted)).
			Bold(true)

	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colAccent)).
			Bold(true)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colFaint)).
			Padding(0, 1)

	daemonOnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colSuccess)).
			Bold(true)

	daemonOffStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colMuted))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colSuccess))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colWarn))
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
		m.viewport = m.makeViewport(m.content)
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
			m.viewport = m.makeViewport(m.content)
			m.current = contentView
		}
	case "r":
		if m.history != nil {
			return m, m.restoreLatestCmd()
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
			return m, m.restoreLatestCmd()
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

	b.WriteString(m.renderHeader("envault", "tracked files"))

	// Column header
	b.WriteString(headerStyle.Render(fmt.Sprintf("  %-4s %-58s %8s  %s", "#", "FILE", "VERSIONS", "LAST BACKUP")) + "\n")

	// Scroll window
	visibleRows := m.height - 6 // header + rule + columns + status bar + padding
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
		line := fmt.Sprintf("%-4s %-58s %8d  %s", num, path, len(f.Snapshots), format.TimeAgo(last.Timestamp))
		b.WriteString(m.renderRow(line, i == m.cursor))
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
		fmt.Sprintf("%d files%s", len(m.files), scrollPart),
		renderHints(
			hint{"s", "scan"}, hint{"d", "daemon"}, hint{"w", "watch"},
			hint{"e", "export"}, hint{"i", "import"}, hint{"↵", "open"}, hint{"q", "quit"},
		),
	))

	return b.String()
}

// ── History view ──────────────────────────────────────────────────────────────

func (m model) viewHistory() string {
	if m.history == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(m.renderHeader("envault", format.ShortenPath(m.history.FilePath)))

	// Column header
	b.WriteString(headerStyle.Render(fmt.Sprintf("  %-4s %-14s %-18s %8s  %s", "#", "ID", "DATE", "SIZE", "COMMENT")) + "\n")

	visibleRows := m.height - 6
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
		line := fmt.Sprintf("%-4d %-14s %-18s %8s  %s",
			i+1,
			snap.ID[:12],
			snap.Timestamp.Local().Format("2006-01-02 15:04"),
			format.HumanSize(snap.Size),
			snap.Comment,
		)
		b.WriteString(m.renderRow(line, i == m.hCursor))
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
		fmt.Sprintf("%d versions%s", len(m.history.Snapshots), scrollPart),
		renderHints(
			hint{"r", "restore"}, hint{"R", "restore to…"}, hint{"↵", "view"},
			hint{"esc", "back"}, hint{"q", "quit"},
		),
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

	b.WriteString(m.renderHeader(
		"envault",
		format.ShortenPath(m.history.FilePath),
		fmt.Sprintf("v%d", m.hCursor+1),
	))
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %s  •  %s  •  %s",
		snap.ID[:12],
		snap.Timestamp.Local().Format("2006-01-02 15:04:05"),
		format.HumanSize(snap.Size),
	)) + "\n")

	// Viewport content
	b.WriteString(borderStyle.Render(m.viewport.View()) + "\n")

	// Status bar
	var hints string
	if m.copiedNotice != "" {
		hints = successStyle.Render(m.copiedNotice)
	} else {
		hints = renderHints(
			hint{"c", "copy"}, hint{"r", "restore"}, hint{"R", "restore to…"},
			hint{"↑↓", "scroll"}, hint{"esc", "back"}, hint{"q", "quit"},
		)
	}
	b.WriteString(m.buildStatusBar(fmt.Sprintf("%.0f%%", m.viewport.ScrollPercent()*100), hints))

	return b.String()
}

// ── Input view ────────────────────────────────────────────────────────────────

func (m model) viewInput() string {
	var b strings.Builder

	b.WriteString(m.renderHeader("envault"))
	b.WriteString("\n")

	b.WriteString(brandStyle.Render("  "+m.inputPrompt) + "\n\n")
	b.WriteString("  " + m.textInput.View() + "\n\n")

	if m.inputAction == inputConfirmOverwrite {
		b.WriteString(warnStyle.Render("  ⚠  This will overwrite all existing vault data.") + "\n")
	}

	rendered := strings.Count(b.String(), "\n")
	for rendered < m.height-2 {
		b.WriteString("\n")
		rendered++
	}
	b.WriteString(m.buildStatusBar("", renderHints(hint{"↵", "confirm"}, hint{"esc", "cancel"})))

	return b.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// hint is a single key/action pair shown in the status bar.
type hint struct{ key, action string }

// renderHints formats key/action pairs with the key in the accent color,
// joined by faint middots — the modern command-bar look.
func renderHints(hints ...hint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = keyStyle.Render(h.key) + dimStyle.Render(" "+h.action)
	}
	return strings.Join(parts, dimStyle.Render("  ·  "))
}

// renderHeader draws the breadcrumb title (amber brand, then path segments)
// followed by a full-width hairline rule.
func (m model) renderHeader(crumbs ...string) string {
	parts := make([]string, len(crumbs))
	for i, c := range crumbs {
		if i == 0 {
			parts[i] = brandStyle.Render("🔒 " + c)
		} else {
			parts[i] = crumbStyle.Render(c)
		}
	}
	width := max(m.width, 1)
	rule := ruleStyle.Render(strings.Repeat("─", width))
	return strings.Join(parts, ruleStyle.Render(" › ")) + "\n" + rule + "\n"
}

// renderRow draws one list row with an accent gutter bar when selected.
func (m model) renderRow(line string, selected bool) string {
	if selected {
		return gutterStyle.Render("▌ ") + selectedStyle.Render(line) + "\n"
	}
	return "  " + normalStyle.Render(line) + "\n"
}

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

	content := " " + mainText + daemonStr
	if countPart != "" {
		content = keyStyle.Render(countPart) + dimStyle.Render("  ·  ") + mainText + daemonStr
	}
	return statusBarStyle.Render(content)
}

// makeViewport builds a viewport sized to the current window, pre-filled with content.
func (m model) makeViewport(content string) viewport.Model {
	const headerHeight = 4 // breadcrumb + rule + meta line + border top
	const footerHeight = 2 // border bottom + status bar
	vpHeight := m.height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	vp := viewport.New(m.width-4, vpHeight)
	vp.SetContent(content)
	return vp
}

// restoreLatestCmd restores the currently-selected snapshot to its original path.
func (m model) restoreLatestCmd() tea.Cmd {
	snap := m.history.Snapshots[m.hCursor]
	filePath := m.history.FilePath
	s := m.store
	return func() tea.Msg {
		err := s.RestoreSnapshot(filePath, snap.ID)
		return restoreDoneMsg{path: filePath, err: err}
	}
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
