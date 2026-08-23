// Package tui is the interactive browser opened by 'envault' and 'envault ui'.
//
// It is a three-view stack — files, that file's versions, that version's
// content — plus a modal prompt that any view can open and return from. The
// code is split so each concern stays findable: model here, messages and
// background work in messages.go, key handling in update.go, rendering in
// view.go.
package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
)

// view is which screen is on top of the stack.
type view int

const (
	fileListView view = iota
	historyView
	contentView
	promptView
)

// promptKind is what the modal prompt is currently asking for. It decides
// which command runs when the user presses enter.
type promptKind int

const (
	promptWatchDir promptKind = iota
	promptExport
	promptImport
	promptRestoreTo
	promptConfirmOverwrite
)

type model struct {
	store *store.Store

	files   []store.FileHistory
	cursor  int
	history *store.FileHistory
	hCursor int

	// The archive shelf: parked histories shown in place of the live list
	// while showArchived is set.
	archived     []store.FileHistory
	showArchived bool

	current view
	width   int
	height  int
	ready   bool

	content  string
	viewport viewport.Model

	daemonRunning bool
	daemonPID     int

	statusMsg    string
	copiedNotice string

	// Modal prompt state.
	prompt      promptKind
	promptLabel string
	input       textinput.Model
	returnTo    view

	// Carried across a prompt: which snapshot to restore, which zip to import.
	pendingSnapshotID string
	pendingImportPath string
}

// Run opens the browser. With nothing tracked there is nothing to browse, so
// it says so and exits rather than showing an empty frame.
func Run() error {
	s, err := store.NewStore()
	if err != nil {
		return err
	}
	files, err := s.ListTrackedFiles()
	if err != nil {
		return err
	}
	archived, err := s.ListArchived()
	if err != nil {
		return err
	}
	if len(files) == 0 && len(archived) == 0 {
		fmt.Println("No .env files tracked yet. Run 'envault scan' first.")
		return nil
	}

	running, pid := daemon.IsRunning()
	m := model{
		store:         s,
		files:         files,
		archived:      archived,
		current:       fileListView,
		daemonRunning: running,
		daemonPID:     pid,
	}

	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }

// listing is the slice the file list view currently shows.
func (m model) listing() []store.FileHistory {
	if m.showArchived {
		return m.archived
	}
	return m.files
}

// openPrompt switches to the modal prompt, remembering which view to return to.
func (m model) openPrompt(kind promptKind, label, initial string, returnTo view) (model, tea.Cmd) {
	ti := textinput.New()
	ti.SetValue(initial)
	if initial == "" {
		ti.Placeholder = "enter a path"
	}
	ti.Width = max(m.width-20, 40)
	ti.CharLimit = 512
	ti.Focus()

	m.input = ti
	m.prompt = kind
	m.promptLabel = label
	m.returnTo = returnTo
	m.current = promptView
	return m, textinput.Blink
}

// selectedSnapshot is the snapshot the history and content views are pointed
// at, or nil when no file is open.
func (m model) selectedSnapshot() *store.Snapshot {
	if m.history == nil || m.hCursor >= len(m.history.Snapshots) {
		return nil
	}
	return &m.history.Snapshots[m.hCursor]
}

// makeViewport sizes the content pane to the window, leaving room for the
// breadcrumb, rule, metadata line, box border and status bar.
func (m model) makeViewport(content string) viewport.Model {
	const chrome = 6
	vp := viewport.New(max(m.width-4, 1), max(m.height-chrome, 1))
	vp.SetContent(content)
	return vp
}

func errorIsVaultExists(err error) bool {
	return err != nil && errors.Is(err, transfer.ErrVaultExists)
}
