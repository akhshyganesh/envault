// Package setup provides the interactive installation wizard for envault.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type step int

const (
	stepDirs step = iota
	stepInterval
	stepInstall
)

type dirOption struct {
	path     string
	selected bool
}

type intervalOption struct {
	label   string
	seconds int
}

var intervalOptions = []intervalOption{
	{"Every minute", 60},
	{"Every 5 minutes", 300},
	{"Every 15 minutes", 900},
	{"Every 30 minutes", 1800},
}

// progressLine is a single line of install output.
type progressLine struct {
	text string
	warn bool
}

// installMsg carries progress from the install goroutine to the TUI.
type installMsg struct {
	line *progressLine // nil means no new line
	done bool
	err  error
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)

	stepLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("57")).
			Bold(true)

	activeRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Bold(true)

	checkedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))
)

// ── Model ─────────────────────────────────────────────────────────────────────

type model struct {
	width  int
	height int
	step   step

	// step 1 — directory selection
	dirs    []dirOption
	dCursor int
	input   textinput.Model
	typing  bool

	// step 2 — interval selection
	iCursor int

	// step 3 — install progress
	lines     []progressLine
	installCh chan installMsg
	done      bool
	err       error
}

func newModel() model {
	ti := textinput.New()
	ti.Placeholder = "~/path/to/directory"
	ti.CharLimit = 256

	return model{
		step:  stepDirs,
		dirs:  detectDirs(),
		input: ti,
	}
}

// detectDirs returns common project directories that exist on disk.
// All detected directories are pre-selected. Falls back to home if none found.
func detectDirs() []dirOption {
	home, _ := os.UserHomeDir()

	candidates := []string{
		"projects", "work", "dev", "code", "src",
		"repos", "workspace", "Documents", "Developer", "Sites",
	}

	var opts []dirOption
	for _, name := range candidates {
		path := filepath.Join(home, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			opts = append(opts, dirOption{path: path, selected: true})
		}
	}

	if len(opts) == 0 {
		opts = append(opts, dirOption{path: home, selected: true})
	}
	return opts
}

func (m model) selectedDirs() []string {
	var out []string
	for _, d := range m.dirs {
		if d.selected {
			out = append(out, d.path)
		}
	}
	return out
}

func (m model) hasDir(abs string) bool {
	for _, d := range m.dirs {
		if d.path == abs {
			return true
		}
	}
	return false
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return nil
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case installMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, nil
		}
		if msg.line != nil {
			m.lines = append(m.lines, *msg.line)
		}
		if msg.done {
			m.done = true
			return m, nil
		}
		return m, listenForInstall(m.installCh)

	case tea.KeyMsg:
		switch m.step {
		case stepDirs:
			return m.updateDirs(msg)
		case stepInterval:
			return m.updateInterval(msg)
		case stepInstall:
			// Block all keys while installing; quit when done.
			if m.done {
				return m, tea.Quit
			}
			return m, nil
		}
	}

	// Forward tick/blink events to the text input while typing.
	if m.typing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) updateDirs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Typing mode (custom path input) ───────────────────────────────────────
	if m.typing {
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.input.Value())
			if val != "" {
				if abs, err := format.ExpandPath(val); err == nil {
					if info, err := os.Stat(abs); err == nil && info.IsDir() && !m.hasDir(abs) {
						m.dirs = append(m.dirs, dirOption{path: abs, selected: true})
					}
				}
			}
			m.typing = false
			m.input.Reset()
			m.input.Blur()
			m.dCursor = len(m.dirs)
		case "esc":
			m.typing = false
			m.input.Reset()
			m.input.Blur()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// ── Normal navigation mode ────────────────────────────────────────────────
	totalRows := len(m.dirs) + 1 // dirs + "add custom" row

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.dCursor > 0 {
			m.dCursor--
		}
	case "down", "j":
		if m.dCursor < totalRows-1 {
			m.dCursor++
		}
	case " ":
		if m.dCursor < len(m.dirs) {
			m.dirs[m.dCursor].selected = !m.dirs[m.dCursor].selected
		}
	case "enter", "l", "right":
		if m.dCursor == len(m.dirs) {
			// Enter typing mode for custom path
			m.typing = true
			m.input.Focus()
			return m, textinput.Blink
		}
		if len(m.selectedDirs()) > 0 {
			m.step = stepInterval
		}
	}
	return m, nil
}

func (m model) updateInterval(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.iCursor > 0 {
			m.iCursor--
		}
	case "down", "j":
		if m.iCursor < len(intervalOptions)-1 {
			m.iCursor++
		}
	case "enter", "l", "right", " ":
		ch := make(chan installMsg, 32)
		m.installCh = ch
		m.step = stepInstall
		go runInstall(ch, m.selectedDirs(), intervalOptions[m.iCursor].seconds)
		return m, listenForInstall(ch)
	case "esc", "h", "left":
		m.step = stepDirs
	}
	return m, nil
}

// ── Install runner ────────────────────────────────────────────────────────────

func listenForInstall(ch chan installMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func runInstall(ch chan installMsg, dirs []string, intervalSecs int) {
	ok := func(text string) { ch <- installMsg{line: &progressLine{text: text}} }
	warn := func(text string) { ch <- installMsg{line: &progressLine{text: text, warn: true}} }
	fail := func(err error) { ch <- installMsg{err: err, done: true} }

	// Save config
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.WatchDirs = dirs
	cfg.ScanIntervalSecs = intervalSecs
	if err := cfg.Save(); err != nil {
		fail(fmt.Errorf("saving config: %w", err))
		return
	}
	ok(fmt.Sprintf("Config saved — watching %d %s", len(dirs), plural(len(dirs), "directory", "directories")))

	// Initial scan
	s, err := store.NewStore()
	if err != nil {
		fail(fmt.Errorf("opening store: %w", err))
		return
	}

	totalNew := 0
	for _, dir := range dirs {
		res, scanErr := scanner.ScanDirectory(dir, s)
		if scanErr != nil {
			warn(fmt.Sprintf("Could not scan %s: %v", format.ShortenPath(dir), scanErr))
			continue
		}
		totalNew += res.Backed
		ok(fmt.Sprintf("%-38s  %d %s, %d new",
			format.ShortenPath(dir),
			len(res.Found),
			plural(len(res.Found), "file", "files"),
			res.Backed,
		))
	}

	// Install OS service
	if err := daemon.Install(); err != nil {
		warn(fmt.Sprintf("Service install: %v", err))
	} else {
		ok("Startup service installed — runs automatically on login")
	}

	ch <- installMsg{done: true}
}

func plural(n int, sing, plur string) string {
	if n == 1 {
		return sing
	}
	return plur
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	var b strings.Builder

	// Header
	title := titleStyle.Render(" 🔒 envault setup ")
	var label string
	switch m.step {
	case stepDirs:
		label = stepLabelStyle.Render("  Step 1 of 3 — Watch Directories")
	case stepInterval:
		label = stepLabelStyle.Render("  Step 2 of 3 — Scan Interval")
	case stepInstall:
		label = stepLabelStyle.Render("  Step 3 of 3 — Installing")
	}
	b.WriteString(title + label + "\n\n")

	switch m.step {
	case stepDirs:
		b.WriteString(m.viewDirs())
	case stepInterval:
		b.WriteString(m.viewInterval())
	case stepInstall:
		b.WriteString(m.viewInstall())
	}

	return b.String()
}

func (m model) viewDirs() string {
	var b strings.Builder

	b.WriteString(dimStyle.Render("  Which directories should envault watch for .env files?") + "\n\n")

	for i, d := range m.dirs {
		cursor := "  "
		if i == m.dCursor && !m.typing {
			cursor = cursorStyle.Render("❯ ")
		}
		box := dimStyle.Render("[ ]")
		if d.selected {
			box = checkedStyle.Render("[✓]")
		}
		line := fmt.Sprintf("%s%s  %s", cursor, box, format.ShortenPath(d.path))
		if i == m.dCursor && !m.typing {
			b.WriteString(activeRowStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// "Add custom path" row
	addCursor := "  "
	if m.dCursor == len(m.dirs) && !m.typing {
		addCursor = cursorStyle.Render("❯ ")
	}
	if m.typing {
		b.WriteString(fmt.Sprintf("  %s  %s\n", dimStyle.Render("[+]"), m.input.View()))
	} else {
		addLine := fmt.Sprintf("%s%s  Add a custom path...", addCursor, dimStyle.Render("[+]"))
		if m.dCursor == len(m.dirs) {
			b.WriteString(activeRowStyle.Render(addLine))
		} else {
			b.WriteString(dimStyle.Render(addLine))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	selected := m.selectedDirs()
	switch {
	case m.typing:
		b.WriteString(hintStyle.Render("  Enter to add · Esc to cancel"))
	case len(selected) == 0:
		b.WriteString(hintStyle.Render("  Space to select at least one directory"))
	default:
		b.WriteString(hintStyle.Render(fmt.Sprintf(
			"  Space toggle · Enter/→ continue  (%d selected)", len(selected),
		)))
	}
	b.WriteString("\n")

	return b.String()
}

func (m model) viewInterval() string {
	var b strings.Builder

	b.WriteString(dimStyle.Render("  How often should envault scan for changes?") + "\n\n")

	for i, opt := range intervalOptions {
		cursor := "  "
		if i == m.iCursor {
			cursor = cursorStyle.Render("❯ ")
		}
		radio := dimStyle.Render("( )")
		if i == m.iCursor {
			radio = checkedStyle.Render("(●)")
		}
		label := opt.label
		if i == 0 {
			label += dimStyle.Render("   ← recommended")
		}
		line := fmt.Sprintf("%s%s  %s", cursor, radio, label)
		if i == m.iCursor {
			b.WriteString(activeRowStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  ↑↓ navigate · Enter confirm · ← back"))
	b.WriteString("\n")

	return b.String()
}

func (m model) viewInstall() string {
	var b strings.Builder

	for _, line := range m.lines {
		if line.warn {
			b.WriteString(warnStyle.Render("  ⚠  ") + line.text + "\n")
		} else {
			b.WriteString(successStyle.Render("  ✓  ") + line.text + "\n")
		}
	}

	if !m.done {
		b.WriteString(dimStyle.Render("  ...") + "\n")
		return b.String()
	}

	// Completion
	b.WriteString("\n")
	b.WriteString(dividerStyle.Render("  "+strings.Repeat("─", 50)) + "\n\n")

	if m.err != nil {
		b.WriteString(warnStyle.Render("  ✗  ") + m.err.Error() + "\n")
	} else {
		b.WriteString(successStyle.Render("  ✓  All done! ") +
			"envault is running in the background.\n\n")
		b.WriteString("  " + dimStyle.Render("envault list") + "   — see your backed-up files\n")
		b.WriteString("  " + dimStyle.Render("envault ui  ") + "   — browse them interactively\n")
	}

	b.WriteString("\n" + hintStyle.Render("  Press any key to exit") + "\n")

	return b.String()
}

// Run launches the interactive setup wizard.
func Run() error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
