package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectDirsOffersOnlyExistingFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"projects", "work"} {
		if err := os.MkdirAll(filepath.Join(home, name), 0700); err != nil {
			t.Fatal(err)
		}
	}

	dirs := detectDirs()
	if len(dirs) != 2 {
		t.Fatalf("detected %d directories, want 2: %+v", len(dirs), dirs)
	}
	for _, d := range dirs {
		if !d.selected {
			t.Errorf("%s was not pre-selected", d.path)
		}
		if _, err := os.Stat(d.path); err != nil {
			t.Errorf("offered a directory that does not exist: %s", d.path)
		}
	}
}

// With no recognisable project folders the wizard must still offer something,
// or step 1 cannot be completed at all.
func TestDetectDirsFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dirs := detectDirs()
	if len(dirs) != 1 || dirs[0].path != home {
		t.Fatalf("want the home directory as a fallback, got %+v", dirs)
	}
}

func TestSelectedDirsReflectsToggles(t *testing.T) {
	m := model{dirs: []dirChoice{
		{path: "/a", selected: true},
		{path: "/b", selected: false},
		{path: "/c", selected: true},
	}}

	if got := strings.Join(m.selectedDirs(), ","); got != "/a,/c" {
		t.Fatalf("selectedDirs = %q, want /a,/c", got)
	}
	if !m.hasDir("/b") {
		t.Error("hasDir should see unselected entries too")
	}
	if m.hasDir("/nope") {
		t.Error("hasDir found a directory that was never added")
	}
}

func TestEveryStepRenders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	base := newModel()
	base.width = 100
	base.height = 30

	for _, s := range []step{stepDirs, stepInterval, stepInstall} {
		m := base
		m.step = s
		out := m.View()
		if out == "" {
			t.Errorf("step %d rendered nothing", s)
		}
		if !strings.Contains(out, "envault setup") {
			t.Errorf("step %d is missing the header", s)
		}
	}
}

func TestInstallStepShowsProgressThenTheSummary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := newModel()
	m.width, m.height = 100, 30
	m.step = stepInstall
	m.lines = []progressLine{
		{text: "Config saved"},
		{text: "Startup service: not supported here", warn: true},
	}

	running := m.View()
	if !strings.Contains(running, "Config saved") || !strings.Contains(running, "working") {
		t.Errorf("in-progress view is wrong:\n%s", running)
	}

	m.done = true
	finished := m.View()
	if strings.Contains(finished, "working") {
		t.Errorf("finished view still says it is working:\n%s", finished)
	}
	if !strings.Contains(finished, "envault list") {
		t.Errorf("finished view does not say what to do next:\n%s", finished)
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "file", "files"); got != "file" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(0, "file", "files"); got != "files" {
		t.Errorf("plural(0) = %q", got)
	}
	if got := plural(2, "file", "files"); got != "files" {
		t.Errorf("plural(2) = %q", got)
	}
}

func TestIntervalChoicesAreSaneAndRecommendTheShortest(t *testing.T) {
	if len(intervalChoices) == 0 {
		t.Fatal("no scan intervals offered")
	}
	for i, c := range intervalChoices {
		if c.seconds < 1 {
			t.Errorf("choice %d has a non-positive interval: %+v", i, c)
		}
		if i > 0 && c.seconds <= intervalChoices[i-1].seconds {
			t.Errorf("choices are not in increasing order at %d: %+v", i, intervalChoices)
		}
	}
}
