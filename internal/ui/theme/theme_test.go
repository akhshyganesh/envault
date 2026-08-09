package theme

import (
	"strings"
	"testing"
)

func TestRuleSpansTheWidth(t *testing.T) {
	if got := strings.Count(Rule(20), GlyphRule); got != 20 {
		t.Errorf("Rule(20) drew %d glyphs, want 20", got)
	}
	// A zero or negative width comes from a terminal that has not reported its
	// size yet; it must not produce an empty or panicking render.
	if Rule(0) == "" || Rule(-5) == "" {
		t.Error("Rule on a zero width rendered nothing")
	}
}

func TestBreadcrumbJoinsSegments(t *testing.T) {
	got := Breadcrumb("envault", "~/app/.env", "v3")
	for _, want := range []string{"envault", "~/app/.env", "v3", GlyphCrumb} {
		if !strings.Contains(got, want) {
			t.Errorf("Breadcrumb is missing %q: %s", want, got)
		}
	}
	if single := Breadcrumb("envault"); strings.Contains(single, GlyphCrumb) {
		t.Errorf("a single segment should carry no separator: %s", single)
	}
}

func TestHintsShowKeysAndActions(t *testing.T) {
	got := Hints(Hint{Key: "s", Action: "scan"}, Hint{Key: "q", Action: "quit"})
	for _, want := range []string{"s", "scan", "q", "quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("Hints is missing %q: %s", want, got)
		}
	}
	if Hints() != "" {
		t.Error("Hints with no pairs should render nothing")
	}
}

func TestTruncateKeepsTheTail(t *testing.T) {
	got := Truncate("/home/tester/very/deep/project/.env", 12)
	if len([]rune(got)) != 12 {
		t.Fatalf("Truncate returned %d runes, want 12: %q", len([]rune(got)), got)
	}
	if !strings.HasPrefix(got, GlyphEllipse) {
		t.Errorf("Truncate did not mark the cut: %q", got)
	}
	// The end of a path is what identifies it, so it must survive.
	if !strings.HasSuffix(got, ".env") {
		t.Errorf("Truncate dropped the tail: %q", got)
	}
	if short := Truncate("short", 20); short != "short" {
		t.Errorf("Truncate shortened a string that fits: %q", short)
	}
}

// The glyphs are chosen to be one cell wide in any terminal; an emoji would
// occupy two and break every box-drawn layout.
func TestGlyphsAreSingleRunes(t *testing.T) {
	for name, glyph := range map[string]string{
		"OK": GlyphOK, "Bad": GlyphBad, "Warn": GlyphWarn, "Cursor": GlyphCursor,
		"Gutter": GlyphGutter, "Crumb": GlyphCrumb, "Sep": GlyphSep, "Enter": GlyphEnter,
		"Rule": GlyphRule, "On": GlyphOn, "Off": GlyphOff, "Ellipse": GlyphEllipse,
	} {
		if n := len([]rune(glyph)); n != 1 {
			t.Errorf("Glyph%s is %d runes, want 1 — emoji break box-drawn layouts", name, n)
		}
	}
}
