package setup

import (
	"fmt"
	"strings"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/ui/theme"
)

func (m model) View() string {
	var b strings.Builder

	b.WriteString(theme.Brand.Render("envault setup") + theme.Separator() + theme.Dim.Render(m.stepLabel()) + "\n")
	b.WriteString(theme.Rule(m.width) + "\n\n")

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

func (m model) stepLabel() string {
	switch m.step {
	case stepDirs:
		return "Step 1 of 3 — Watch directories"
	case stepInterval:
		return "Step 2 of 3 — Scan interval"
	default:
		return "Step 3 of 3 — Setting up"
	}
}

func (m model) viewDirs() string {
	var b strings.Builder
	b.WriteString(theme.Dim.Render("  Which directories should envault watch for .env files?") + "\n\n")

	for i, d := range m.dirs {
		box := theme.Dim.Render("[ ]")
		if d.selected {
			box = theme.Good.Render("[" + theme.GlyphOK + "]")
		}
		active := i == m.dCursor && !m.typing
		b.WriteString(row(active, box+"  "+format.ShortenPath(d.path)) + "\n")
	}

	// The "add a custom path" row becomes the text field while typing.
	if m.typing {
		b.WriteString(fmt.Sprintf("  %s  %s\n", theme.Dim.Render("[+]"), m.input.View()))
	} else {
		active := m.dCursor == len(m.dirs)
		b.WriteString(row(active, theme.Dim.Render("[+]")+"  Add a custom path") + "\n")
	}

	b.WriteString("\n")
	switch selected := m.selectedDirs(); {
	case m.typing:
		b.WriteString(theme.Dim.Render("  Enter to add" + theme.Separator() + "Esc to cancel"))
	case len(selected) == 0:
		b.WriteString(theme.Dim.Render("  Press Space to select at least one directory"))
	default:
		b.WriteString(theme.Hints(
			theme.Hint{Key: "space", Action: "toggle"},
			theme.Hint{Key: theme.GlyphEnter, Action: fmt.Sprintf("continue (%d selected)", len(selected))},
		))
	}
	return b.String() + "\n"
}

func (m model) viewInterval() string {
	var b strings.Builder
	b.WriteString(theme.Dim.Render("  How often should envault check for changes?") + "\n\n")

	for i, choice := range intervalChoices {
		radio := theme.Dim.Render("( )")
		if i == m.iCursor {
			radio = theme.Good.Render("(" + theme.GlyphOn + ")")
		}
		label := choice.label
		if i == 0 {
			label += theme.Dim.Render("   recommended")
		}
		b.WriteString(row(i == m.iCursor, radio+"  "+label) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(theme.Hints(
		theme.Hint{Key: "↑↓", Action: "choose"},
		theme.Hint{Key: theme.GlyphEnter, Action: "confirm"},
		theme.Hint{Key: "esc", Action: "back"},
	))
	return b.String() + "\n"
}

func (m model) viewInstall() string {
	var b strings.Builder

	for _, line := range m.lines {
		mark := theme.Good.Render("  " + theme.GlyphOK + "  ")
		if line.warn {
			mark = theme.Bad.Render("  " + theme.GlyphWarn + "  ")
		}
		b.WriteString(mark + line.text + "\n")
	}

	if !m.done {
		b.WriteString(theme.Dim.Render("  working…") + "\n")
		return b.String()
	}

	b.WriteString("\n" + theme.RuleText.Render("  "+strings.Repeat(theme.GlyphRule, 50)) + "\n\n")
	if m.err != nil {
		b.WriteString(theme.Bad.Render("  "+theme.GlyphBad+"  ") + m.err.Error() + "\n")
	} else {
		b.WriteString(theme.Good.Render("  "+theme.GlyphOK+"  All set. ") + "envault is watching in the background.\n\n")
		b.WriteString("  " + theme.Key.Render("envault status") + theme.Dim.Render("   is it running, and will it start at login") + "\n")
		b.WriteString("  " + theme.Key.Render("envault list  ") + theme.Dim.Render("   see what is backed up") + "\n")
		b.WriteString("  " + theme.Key.Render("envault ui    ") + theme.Dim.Render("   browse it interactively") + "\n")
	}

	return b.String() + "\n" + theme.Dim.Render("  Press any key to exit") + "\n"
}

// row renders a selectable line, with the accent cursor when it is active.
func row(active bool, text string) string {
	if active {
		return theme.Gutter.Render(theme.GlyphCursor+" ") + theme.Selected.Render(text)
	}
	return "  " + text
}
