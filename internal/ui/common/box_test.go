package common

import (
	"strings"
	"testing"
)

func TestTitledBox(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		content   string
		innerW    int
		innerH    int
		wantTitle bool // title text should appear in the border
		wantLines int  // total rendered rows (top + innerH + bottom)
	}{
		{name: "fits title", title: "output", content: "a\nb", innerW: 20, innerH: 4, wantTitle: true, wantLines: 6},
		{name: "pads short content", title: "x", content: "only one line", innerW: 20, innerH: 5, wantTitle: true, wantLines: 7},
		{name: "truncates long content", title: "x", content: "1\n2\n3\n4\n5\n6", innerW: 20, innerH: 3, wantTitle: true, wantLines: 5},
		{name: "too narrow drops title", title: "verylongtitle", content: "", innerW: 2, innerH: 2, wantTitle: false, wantLines: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := TitledBox(tt.title, tt.content, tt.innerW, tt.innerH, false)
			if got := strings.Count(out, "\n") + 1; got != tt.wantLines {
				t.Errorf("line count = %d, want %d", got, tt.wantLines)
			}
			if got := strings.Contains(out, tt.title); got != tt.wantTitle {
				t.Errorf("contains title %q = %v, want %v", tt.title, got, tt.wantTitle)
			}
		})
	}
}
