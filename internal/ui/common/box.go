// Package common holds reusable lipgloss render helpers shared across panels.
package common

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// TitledBox draws a rounded box with the title inlined into the top border,
// lazydocker style. lipgloss has no native border title, so we build it by hand.
// content is padded/truncated to exactly innerW x innerH.
func TitledBox(title, content string, innerW, innerH int, focused bool) string {
	borderC, titleC := Muted, Title
	if focused {
		borderC, titleC = Accent, Accent
	}
	bs := lipgloss.NewStyle().Foreground(borderC)
	ts := lipgloss.NewStyle().Foreground(titleC).Bold(true)

	span := innerW + 2 // chars between corners (1 space of padding each side)

	label := " " + title + " "
	fill := span - 1 - lipgloss.Width(label) // -1 for the leading dash
	var top string
	if fill < 0 {
		top = bs.Render("╭" + strings.Repeat("─", span) + "╮") // too narrow for the title
	} else {
		top = bs.Render("╭─") + ts.Render(label) + bs.Render(strings.Repeat("─", fill)+"╮")
	}
	bottom := bs.Render("╰" + strings.Repeat("─", span) + "╯")
	side := bs.Render("│")

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	lines = lines[:innerH]
	cell := lipgloss.NewStyle().Width(innerW)

	var b strings.Builder
	b.WriteString(top + "\n")
	for _, ln := range lines {
		b.WriteString(side + " " + cell.Render(ln) + " " + side + "\n")
	}
	b.WriteString(bottom)
	return b.String()
}
