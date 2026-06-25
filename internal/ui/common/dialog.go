package common

import "charm.land/lipgloss/v2"

// Dialog is a centered popup box that sizes to its content, lazydocker style.
// The zero value fits the content exactly; set Width/Height to pin a dimension,
// MinWidth to floor an otherwise-narrow auto width. It wraps the existing
// TitledBox (the box) and Center (the placement) so callers stop hand-tuning
// per-modal size constants.
type Dialog struct {
	Title    string
	Width    int // inner width; 0 = fit content
	Height   int // inner height; 0 = fit content
	MinWidth int // floor for the auto width (ignored when Width is set)
}

// Render draws the dialog box around content, auto-sizing any dimension left 0.
func (d Dialog) Render(content string) string {
	w, h := d.Width, d.Height
	if w == 0 {
		w = max(lipgloss.Width(content), d.MinWidth)
	}
	if h == 0 {
		h = lipgloss.Height(content)
	}
	return TitledBox(d.Title, content, w, h, true)
}
