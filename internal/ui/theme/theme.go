// Package theme is the single source of envault's terminal identity: a warm
// amber accent on neutral grays, hairline rules, and a status bar whose keys
// are the only coloured thing in it.
//
// The TUI and the setup wizard both render from here, so changing a colour or
// a glyph in one place changes both. Nothing in this package knows what a
// vault is — it only knows how envault looks.
package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// 256-colour palette. Change colours here, never at a call site.
const (
	Accent   = "180" // warm amber: brand, keys, the selected row's gutter
	AccentHi = "215" // brighter amber: the selected row's text
	Text     = "252"
	Muted    = "245"
	Faint    = "240" // rules and tertiary detail
	Success  = "114" // soft green
	Warn     = "215"
	BarBg    = "236"
)

// Glyphs. Deliberately typographic rather than emoji: they render at a
// predictable single-cell width in every terminal, which box-drawing layouts
// depend on.
const (
	GlyphOK      = "✓"
	GlyphBad     = "✗"
	GlyphWarn    = "!"
	GlyphCursor  = "❯"
	GlyphGutter  = "▌"
	GlyphCrumb   = "›"
	GlyphSep     = "·"
	GlyphEnter   = "↵"
	GlyphRule    = "─"
	GlyphOn      = "●"
	GlyphOff     = "○"
	GlyphEllipse = "…"
)

var (
	Brand     = lipgloss.NewStyle().Foreground(lipgloss.Color(Accent)).Bold(true)
	Crumb     = lipgloss.NewStyle().Foreground(lipgloss.Color(Text))
	RuleText  = lipgloss.NewStyle().Foreground(lipgloss.Color(Faint))
	Gutter    = lipgloss.NewStyle().Foreground(lipgloss.Color(Accent)).Bold(true)
	Selected  = lipgloss.NewStyle().Foreground(lipgloss.Color(AccentHi)).Bold(true)
	Normal    = lipgloss.NewStyle().Foreground(lipgloss.Color(Text))
	Dim       = lipgloss.NewStyle().Foreground(lipgloss.Color(Muted))
	Key       = lipgloss.NewStyle().Foreground(lipgloss.Color(Accent)).Bold(true)
	ColumnRow = lipgloss.NewStyle().Foreground(lipgloss.Color(Muted)).Bold(true)
	StatusBar = lipgloss.NewStyle().Foreground(lipgloss.Color(Muted)).Background(lipgloss.Color(BarBg)).Padding(0, 1)
	Box       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(Faint)).Padding(0, 1)
	Good      = lipgloss.NewStyle().Foreground(lipgloss.Color(Success))
	Bad       = lipgloss.NewStyle().Foreground(lipgloss.Color(Warn))
)

// Rule draws the hairline that sits under every header.
func Rule(width int) string {
	return RuleText.Render(strings.Repeat(GlyphRule, max(width, 1)))
}

// Separator is the faint middot that joins hints and breadcrumb segments.
func Separator() string {
	return Dim.Render("  " + GlyphSep + "  ")
}

// Hint is one key/action pair in the status bar.
type Hint struct {
	Key    string
	Action string
}

// Hints renders key/action pairs with only the keys in the accent colour, so
// the eye lands on what is pressable.
func Hints(hints ...Hint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = Key.Render(h.Key) + Dim.Render(" "+h.Action)
	}
	return strings.Join(parts, Separator())
}

// Breadcrumb renders "envault › ~/app/.env › v3" — the first segment is the
// brand, the rest are path context.
func Breadcrumb(segments ...string) string {
	parts := make([]string, len(segments))
	for i, s := range segments {
		if i == 0 {
			parts[i] = Brand.Render(s)
		} else {
			parts[i] = Crumb.Render(s)
		}
	}
	return strings.Join(parts, RuleText.Render(" "+GlyphCrumb+" "))
}

// Header is a breadcrumb over a full-width rule: the top of every view.
func Header(width int, segments ...string) string {
	return Breadcrumb(segments...) + "\n" + Rule(width) + "\n"
}

// Truncate shortens a string to width, marking the cut with an ellipsis. It
// keeps the tail, because the end of a path identifies it better than the start.
func Truncate(s string, width int) string {
	if width <= 1 || len([]rune(s)) <= width {
		return s
	}
	r := []rune(s)
	return GlyphEllipse + string(r[len(r)-(width-1):])
}
