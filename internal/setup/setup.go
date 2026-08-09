// Package setup is the three-step wizard shown on first run and by
// 'envault install': pick directories, pick a scan interval, then watch it
// configure itself.
package setup

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type step int

const (
	stepDirs step = iota
	stepInterval
	stepInstall
)

// dirChoice is one togglable row in step 1.
type dirChoice struct {
	path     string
	selected bool
}

type intervalChoice struct {
	label   string
	seconds int
}

// A minute is first and marked as recommended: scanning is a cheap hash
// comparison, so the default errs toward losing less work.
var intervalChoices = []intervalChoice{
	{"Every minute", 60},
	{"Every 5 minutes", 300},
	{"Every 15 minutes", 900},
	{"Every 30 minutes", 1800},
}

// candidateDirs are the folder names people keep code in. Only those that
// exist are offered, so the list is short and true for this machine.
var candidateDirs = []string{
	"projects", "work", "dev", "code", "src",
	"repos", "workspace", "Documents", "Developer", "Sites",
}

type model struct {
	width  int
	height int
	step   step

	// Step 1
	dirs    []dirChoice
	dCursor int
	input   textinput.Model
	typing  bool

	// Step 2
	iCursor int

	// Step 3
	lines    []progressLine
	progress chan progressMsg
	done     bool
	err      error
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

// detectDirs pre-selects every candidate that exists, falling back to the home
// directory so the wizard is never offering an empty list.
func detectDirs() []dirChoice {
	home, _ := os.UserHomeDir()

	var out []dirChoice
	for _, name := range candidateDirs {
		p := filepath.Join(home, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, dirChoice{path: p, selected: true})
		}
	}
	if len(out) == 0 {
		out = append(out, dirChoice{path: home, selected: true})
	}
	return out
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

func (m model) hasDir(path string) bool {
	for _, d := range m.dirs {
		if d.path == path {
			return true
		}
	}
	return false
}

func (m model) Init() tea.Cmd { return nil }

// Run opens the wizard.
func Run() error {
	_, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run()
	return err
}
