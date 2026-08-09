package tui

import (
	"strings"
	"testing"
	"time"

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
