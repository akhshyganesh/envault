package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/ui/theme"
)

func fixture() model {
	history := store.FileHistory{
		FilePath: "/home/tester/app/.env",
		Snapshots: []store.Snapshot{
			{ID: strings.Repeat("a", 64), Timestamp: time.Now().Add(-2 * time.Hour), Size: 120, Comment: "auto"},
			{ID: strings.Repeat("b", 64), Timestamp: time.Now(), Size: 140, Comment: "auto"},
		},
	}
	m := model{
		files:       []store.FileHistory{history},
		history:     &history,
		hCursor:     1,
		content:     "KEY=value\n",
		width:       100,
		height:      30,
		ready:       true,
		current:     fileListView,
		promptLabel: "Restore to which path?",
	}
	m.viewport = m.makeViewport(m.content)
	return m
}

func TestEveryViewRenders(t *testing.T) {
	m := fixture()
	views := map[string]func() string{
		"fileList": m.viewFileList,
		"history":  m.viewHistory,
		"content":  m.viewContent,
		"prompt":   m.viewPrompt,
	}
	for name, render := range views {
		out := render()
		if out == "" {
			t.Errorf("%s rendered nothing", name)
		}
		if !strings.Contains(out, "envault") {
			t.Errorf("%s is missing the brand mark", name)
		}
	}
}

// The views are rendered before the first WindowSizeMsg arrives.
func TestViewBeforeSizeIsKnown(t *testing.T) {
	if out := (model{}).View(); out == "" {
		t.Fatal("View before ready rendered nothing")
	}
}

func TestSelectedRowGetsTheGutterBar(t *testing.T) {
	m := fixture()
	if !strings.Contains(m.renderRow("row", true), theme.GlyphGutter) {
		t.Error("the selected row has no gutter bar")
	}
	if strings.Contains(m.renderRow("row", false), theme.GlyphGutter) {
		t.Error("an unselected row has a gutter bar")
	}
}

func TestMoveCursorStaysInRange(t *testing.T) {
	cases := []struct {
		key   string
		start int
		count int
		want  int
	}{
		{"down", 0, 3, 1},
		{"j", 2, 3, 2}, // already at the end
		{"up", 1, 3, 0},
		{"k", 0, 3, 0}, // already at the start
		{"g", 2, 3, 0},
		{"G", 0, 3, 2},
		{"G", 0, 0, 0}, // empty list
	}
	for _, c := range cases {
		cursor := c.start
		if !moveCursor(&cursor, c.count, c.key) {
			t.Errorf("moveCursor did not handle %q", c.key)
		}
		if cursor != c.want {
			t.Errorf("%q from %d of %d gave %d, want %d", c.key, c.start, c.count, cursor, c.want)
		}
	}

	cursor := 1
	if moveCursor(&cursor, 3, "q") {
		t.Error("moveCursor claimed a key it does not handle")
	}
	if cursor != 1 {
		t.Errorf("an unhandled key moved the cursor to %d", cursor)
	}
}

func TestWindowKeepsTheCursorVisible(t *testing.T) {
	cases := []struct {
		cursor, count, rows int
		wantStart, wantEnd  int
	}{
		{0, 10, 5, 0, 5},  // top of a long list
		{7, 10, 5, 3, 8},  // scrolled so the cursor is the last row
		{0, 3, 5, 0, 3},   // list shorter than the window
		{9, 10, 5, 5, 10}, // bottom
	}
	for _, c := range cases {
		start, end := window(c.cursor, c.count, c.rows)
		if start != c.wantStart || end != c.wantEnd {
			t.Errorf("window(%d,%d,%d) = %d,%d want %d,%d",
				c.cursor, c.count, c.rows, start, end, c.wantStart, c.wantEnd)
		}
		if c.count > 0 && (c.cursor < start || c.cursor >= end) {
			t.Errorf("window(%d,%d,%d) scrolled the cursor off screen", c.cursor, c.count, c.rows)
		}
	}
}

func TestScrollHintOnlyWhenTheListOverflows(t *testing.T) {
	if got := scrollHint(0, 3, 10); got != "" {
		t.Errorf("scrollHint on a short list = %q, want empty", got)
	}
	if got := scrollHint(2, 40, 10); got != " 3/40" {
		t.Errorf("scrollHint = %q, want ' 3/40'", got)
	}
}

func TestShortIDIsSafeOnShortInput(t *testing.T) {
	if got := shortID(strings.Repeat("f", 64)); len(got) != 12 {
		t.Errorf("shortID length = %d, want 12", len(got))
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID(abc) = %q, want it unchanged", got)
	}
}

func TestSelectedSnapshotHandlesAnEmptyModel(t *testing.T) {
	if (model{}).selectedSnapshot() != nil {
		t.Error("want nil when no file is open")
	}
	m := fixture()
	m.hCursor = 99
	if m.selectedSnapshot() != nil {
		t.Error("want nil when the cursor is past the end")
	}
}

// ── the archive shelf ────────────────────────────────────────────────────────

// newVaultModel builds a model over a real temporary vault with one tracked
// file, returning the file's path.
func newVaultModel(t *testing.T) (model, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	env := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(env, []byte("KEY=value"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SaveSnapshot(env, "auto"); err != nil {
		t.Fatal(err)
	}
	files, err := s.ListTrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	m := fixture()
	m.store = s
	m.files = files
	return m, env
}

func TestTheShelfToggleListsArchivedFiles(t *testing.T) {
	parked := store.FileHistory{
		FilePath:  "/home/tester/old/.env",
		Snapshots: []store.Snapshot{{ID: strings.Repeat("c", 64), Timestamp: time.Now(), Size: 10}},
	}
	m := fixture()
	m.archived = []store.FileHistory{parked}

	out := m.viewFileList()
	if strings.Contains(out, "old") {
		t.Error("archived files leaked into the tracked listing")
	}

	m.showArchived = true
	out = m.viewFileList()
	if !strings.Contains(out, "old/.env") {
		t.Errorf("the shelf does not show the parked file:\n%s", out)
	}
}

func TestToggleKeyFlipsToTheShelf(t *testing.T) {
	m := fixture()

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	next := upd.(model)
	if !next.showArchived {
		t.Fatal("'A' did not open the archive shelf")
	}

	upd, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if next2 := upd.(model); next2.showArchived {
		t.Fatal("'A' did not leave the archive shelf")
	}
}

func TestArchiveKeyParksTheSelectedFile(t *testing.T) {
	m, env := newVaultModel(t)

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("'a' issued no command")
	}
	if msg, ok := cmd().(archiveDoneMsg); !ok || msg.err != nil {
		t.Fatalf("archive command returned %+v, want an error-free archiveDoneMsg", msg)
	}

	next := upd.(model)
	files, _ := next.store.ListTrackedFiles()
	archived, _ := next.store.ListArchived()
	if len(files) != 0 || len(archived) != 1 {
		t.Fatalf("after archiving: %d tracked, %d archived", len(files), len(archived))
	}
	if _, err := os.Stat(env); err != nil {
		t.Errorf("the file on disk was disturbed: %v", err)
	}
}
