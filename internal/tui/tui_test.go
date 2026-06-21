package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/akhshyganesh/envault/internal/store"
)

// fixtureModel builds a ready model with one tracked file and two versions.
func fixtureModel() model {
	snap := store.Snapshot{ID: strings.Repeat("a", 64), Timestamp: time.Now(), Size: 42, Comment: "auto"}
	h := store.FileHistory{FilePath: "/home/u/app/.env", Snapshots: []store.Snapshot{snap, snap}}
	return model{
		files:   []store.FileHistory{h},
		history: &h,
		current: fileListView,
		width:   100,
		height:  30,
		ready:   true,
	}
}

func TestViewsRender(t *testing.T) {
	m := fixtureModel()
	views := map[string]func() string{
		"fileList": m.viewFileList,
		"history":  m.viewHistory,
		"content":  m.viewContent,
	}
	for name, render := range views {
		out := render() // must not panic
		if out == "" {
			t.Errorf("%s view rendered empty", name)
		}
		if !strings.Contains(out, "envault") {
			t.Errorf("%s view missing brand mark", name)
		}
	}
}

func TestRenderRowGutter(t *testing.T) {
	m := fixtureModel()
	selected := m.renderRow("row", true)
	plain := m.renderRow("row", false)
	if !strings.Contains(selected, "▌") {
		t.Error("selected row should have the accent gutter bar")
	}
	if strings.Contains(plain, "▌") {
		t.Error("unselected row should not have the gutter bar")
	}
}
