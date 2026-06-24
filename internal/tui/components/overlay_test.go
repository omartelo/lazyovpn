package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestOverlayPlacesForeground(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA" // 5x3
	got := Overlay(bg, "XX", 1, 1)
	want := "AAAAA\nAXXAA\nAAAAA"
	if got != want {
		t.Errorf("Overlay =\n%q\nwant\n%q", got, want)
	}
}

func TestCenterPlacesInMiddle(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA" // 5x3
	got := Center(bg, "X")      // 1x1 → x=2, y=1
	want := "AAAAA\nAAXAA\nAAAAA"
	if got != want {
		t.Errorf("Center =\n%q\nwant\n%q", got, want)
	}
}

func TestOverlayPreservesWidth(t *testing.T) {
	bg := strings.Repeat("A", 20) + "\n" + strings.Repeat("A", 20)
	got := Center(bg, "modal")
	for _, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w != 20 {
			t.Errorf("line width = %d, want 20 (%q)", w, line)
		}
	}
}
