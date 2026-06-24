package components

import "charm.land/lipgloss/v2"

// Overlay composites fg on top of bg at (x, y) using lipgloss v2's layer
// compositor, keeping bg visible (and styled) around the popup.
func Overlay(bg, fg string, x, y int) string {
	base := lipgloss.NewLayer(bg)
	top := lipgloss.NewLayer(fg).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(base, top).Render()
}

// Center composites fg centered on top of bg — a popup floating over the view.
func Center(bg, fg string) string {
	x := max((lipgloss.Width(bg)-lipgloss.Width(fg))/2, 0)
	y := max((lipgloss.Height(bg)-lipgloss.Height(fg))/2, 0)
	return Overlay(bg, fg, x, y)
}
