// Package model holds the root bubbletea model (ui.go) and the per-panel
// sub-models it composes. Each panel owns its own state and business rules so
// new views can be added without touching the others.
package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

var (
	connectedStyle = lipgloss.NewStyle().Foreground(common.Connected)         // green ● + connected row
	itemSelected   = lipgloss.NewStyle().Foreground(common.Accent).Bold(true) // pink cursor row
	itemNormal     = lipgloss.NewStyle()
)

// rowKind classifies a sidebar row for styling.
type rowKind uint8

const (
	rowPlain     rowKind = iota
	rowCursor            // the highlighted row
	rowConnected         // the live connection (when it is not also the cursor)
)

// Sidebar is the connection-list panel. It renders the discovered configs
// directly — a cursor row, a green ● on the connected one (read live from the
// shared state) — with no bubbles list or delegate behind it. One list in the
// whole app means hand-rolling beats a generic list component; extract one only
// if a second list view ever appears.
type Sidebar struct {
	configs []vpn.Config
	cursor  int // highlighted row
	w, h    int // inner content size (inside the border)
	sh      *shared
}

// NewSidebar builds the sidebar from the discovered configs, reading the live
// connection name from the shared state by pointer.
func NewSidebar(configs []vpn.Config, sh *shared) Sidebar {
	return Sidebar{configs: configs, sh: sh}
}

// SetSize sets the inner content size (inside the border).
func (s *Sidebar) SetSize(w, h int) { s.w, s.h = w, h }

// Move shifts the cursor by delta, clamped to the list.
func (s *Sidebar) Move(delta int) {
	if len(s.configs) == 0 {
		return
	}
	s.cursor = max(0, min(s.cursor+delta, len(s.configs)-1))
}

// AddConfig appends a freshly imported config, skipping it if one with the same
// path is already listed.
func (s *Sidebar) AddConfig(c vpn.Config) {
	for _, existing := range s.configs {
		if existing.Path == c.Path {
			return
		}
	}
	s.configs = append(s.configs, c)
}

// RenameConfig replaces the listed config matching old.Path with updated (a
// rename changes both name and path). No-op if it is not listed.
func (s *Sidebar) RenameConfig(old, updated vpn.Config) {
	for i := range s.configs {
		if s.configs[i].Path == old.Path {
			s.configs[i] = updated
			return
		}
	}
}

// RemoveConfig drops the listed config matching c.Path and clamps the cursor so
// it stays on a valid row (moves up when the last row is removed). No-op if it
// is not listed.
func (s *Sidebar) RemoveConfig(c vpn.Config) {
	for i := range s.configs {
		if s.configs[i].Path == c.Path {
			s.configs = append(s.configs[:i], s.configs[i+1:]...)
			break
		}
	}
	if s.cursor >= len(s.configs) {
		s.cursor = max(0, len(s.configs)-1)
	}
}

// SelectedConfig returns the highlighted config.
func (s Sidebar) SelectedConfig() (vpn.Config, bool) {
	if s.cursor < 0 || s.cursor >= len(s.configs) {
		return vpn.Config{}, false
	}
	return s.configs[s.cursor], true
}

// SelectedName returns the highlighted connection name, or "".
func (s Sidebar) SelectedName() string {
	if c, ok := s.SelectedConfig(); ok {
		return c.Name
	}
	return ""
}

// rowKind classifies row i for styling: the cursor wins over the connected
// marker when they fall on the same row.
func (s Sidebar) rowKind(i int, connected bool) rowKind {
	switch {
	case i == s.cursor:
		return rowCursor
	case connected:
		return rowConnected
	}
	return rowPlain
}

// View renders the bordered panel: one row per config, the ● marker on the
// connected one, the cursor highlight on the selected one.
//
// ponytail: no scroll window — a VPN config list fits the pane in practice. Add
// a cursor-tracking visible-row offset only if a real list ever overflows.
func (s Sidebar) View(focused bool) string {
	title := fmt.Sprintf("connections (%d)", len(s.configs))

	rows := make([]string, len(s.configs))
	for i, c := range s.configs {
		connected := c.Name == s.sh.connected

		marker := "  " // keep names aligned whether or not the dot shows
		if connected {
			marker = connectedStyle.Render("●") + " "
		}

		style := itemNormal
		switch s.rowKind(i, connected) {
		case rowCursor:
			style = itemSelected
		case rowConnected:
			style = connectedStyle
		}

		rows[i] = lipgloss.NewStyle().MaxWidth(s.w).Render(marker + style.Render(c.Name))
	}
	return common.TitledBox(title, strings.Join(rows, "\n"), s.w, s.h, focused)
}
