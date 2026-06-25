package common

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// TitledBox adds 4 columns (border + padding each side) and 2 rows (top+bottom
// border) of chrome around its inner content.
const (
	chromeW = 4
	chromeH = 2
)

func TestPopupRenderSizing(t *testing.T) {
	// "abc" + "de" → widest line 3 cols, 2 lines tall.
	const content = "abc\nde"

	tests := []struct {
		name      string
		dialog    Popup
		wantInalW int // expected inner width
		wantInalH int // expected inner height
	}{
		{"auto fits content", Popup{}, 3, 2},
		{"min width floors auto", Popup{MinWidth: 20}, 20, 2},
		{"min width below content is ignored", Popup{MinWidth: 1}, 3, 2},
		{"pinned width overrides content", Popup{Width: 40}, 40, 2},
		{"pinned width ignores min width", Popup{Width: 40, MinWidth: 80}, 40, 2},
		{"pinned height pads", Popup{Height: 6}, 3, 6},
		{"pinned both", Popup{Width: 30, Height: 5}, 30, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.dialog.Render(content)
			if w := lipgloss.Width(out) - chromeW; w != tt.wantInalW {
				t.Errorf("inner width = %d, want %d", w, tt.wantInalW)
			}
			if h := lipgloss.Height(out) - chromeH; h != tt.wantInalH {
				t.Errorf("inner height = %d, want %d", h, tt.wantInalH)
			}
		})
	}
}
