package common

import "charm.land/lipgloss/v2"

// Popup is an auto-sizing titled box for overlay content, lazydocker style.
// The zero value fits the content exactly; set Width/Height to pin a dimension,
// MinWidth to floor an otherwise-narrow auto width. It wraps TitledBox so
// callers stop hand-tuning per-modal size constants; placing it over the view
// (Center) is the caller's job.
type Popup struct {
	Title    string
	Width    int // inner width; 0 = fit content
	Height   int // inner height; 0 = fit content
	MinWidth int // floor for the auto width (ignored when Width is set)
}

// Render draws the titled box around content, auto-sizing any dimension left 0.
func (d Popup) Render(content string) string {
	w, h := d.Width, d.Height
	if w == 0 {
		w = max(lipgloss.Width(content), d.MinWidth)
	}
	if h == 0 {
		h = lipgloss.Height(content)
	}
	return TitledBox(d.Title, content, w, h, true)
}
