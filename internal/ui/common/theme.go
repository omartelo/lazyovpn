package common

import "charm.land/lipgloss/v2"

// Shared color palette (ANSI 256). Named once here so the same hue is never
// re-spelled as a raw code per panel — change a role's color in one place.
var (
	Accent    = lipgloss.Color("205") // pink: focus / selection
	Connected = lipgloss.Color("42")  // green: live connection
	Warn      = lipgloss.Color("214") // amber: connecting
	Danger    = lipgloss.Color("196") // red: error
	Muted     = lipgloss.Color("240") // gray: idle / unfocused border
	Title     = lipgloss.Color("252") // light: unfocused title
)

// Hint is the faint style for secondary text (footers, field hints).
var Hint = lipgloss.NewStyle().Faint(true)
