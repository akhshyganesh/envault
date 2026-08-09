package setup

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/akhshyganesh/envault/internal/format"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case progressMsg:
		if msg.err != nil {
			m.err, m.done = msg.err, true
			return m, nil
		}
		if msg.line != nil {
			m.lines = append(m.lines, *msg.line)
		}
		if msg.done {
			m.done = true
			return m, nil
		}
		return m, waitForProgress(m.progress)

	case tea.KeyMsg:
		switch m.step {
		case stepDirs:
			return m.onDirsKey(msg)
		case stepInterval:
			return m.onIntervalKey(msg)
		case stepInstall:
			// Installing is not interruptible; any key exits once it is done.
			if m.done {
				return m, tea.Quit
			}
			return m, nil
		}
	}

	// Keep the cursor blinking while a custom path is being typed.
	if m.typing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) onDirsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.typing {
		return m.onCustomPathKey(msg)
	}

	// One row past the directories is the "add a custom path" affordance.
	rows := len(m.dirs) + 1

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.dCursor > 0 {
			m.dCursor--
		}
	case "down", "j":
		if m.dCursor < rows-1 {
			m.dCursor++
		}
	case " ":
		if m.dCursor < len(m.dirs) {
			m.dirs[m.dCursor].selected = !m.dirs[m.dCursor].selected
		}
	case "enter", "l", "right":
		if m.dCursor == len(m.dirs) {
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

// onCustomPathKey handles the inline text field. A path that does not exist is
// silently ignored rather than erroring: the field stays, and the user can see
// their typo.
func (m model) onCustomPathKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if value := strings.TrimSpace(m.input.Value()); value != "" {
			if abs, err := format.ExpandPath(value); err == nil {
				if info, err := os.Stat(abs); err == nil && info.IsDir() && !m.hasDir(abs) {
					m.dirs = append(m.dirs, dirChoice{path: abs, selected: true})
				}
			}
		}
		m.stopTyping()
		m.dCursor = len(m.dirs)
		return m, nil

	case "esc":
		m.stopTyping()
		return m, nil

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m *model) stopTyping() {
	m.typing = false
	m.input.Reset()
	m.input.Blur()
}

func (m model) onIntervalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.iCursor > 0 {
			m.iCursor--
		}
	case "down", "j":
		if m.iCursor < len(intervalChoices)-1 {
			m.iCursor++
		}
	case "esc", "h", "left":
		m.step = stepDirs
	case "enter", "l", "right", " ":
		ch := make(chan progressMsg, 32)
		m.progress = ch
		m.step = stepInstall
		go runInstall(ch, m.selectedDirs(), intervalChoices[m.iCursor].seconds)
		return m, waitForProgress(ch)
	}
	return m, nil
}
