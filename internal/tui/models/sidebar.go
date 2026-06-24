// Package models holds the per-panel bubbletea sub-models and their business rules.
// Each panel owns its own state so new views can be added without touching the others.
package models

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/tui/components"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// item adapts vpn.Config to the bubbles list.
type item vpn.Config

func (i item) Title() string       { return i.Name }
func (i item) Description() string { return i.Path }
func (i item) FilterValue() string { return i.Name }

// Sidebar is the connection list panel.
type Sidebar struct {
	list list.Model
	w, h int // inner content size (inside the border)
}

// NewSidebar builds the sidebar from the discovered configs.
func NewSidebar(configs []vpn.Config) Sidebar {
	items := make([]list.Item, len(configs))
	for i, c := range configs {
		items[i] = item(c)
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false // narrow sidebar: name only

	l := list.New(items, d, 0, 0)
	l.SetShowTitle(false)     // pane title lives in the border now
	l.SetShowStatusBar(false) // count goes in the border title
	l.SetShowHelp(false)

	return Sidebar{list: l}
}

// SetSize sets the inner content size (inside the border).
func (s *Sidebar) SetSize(w, h int) {
	s.w, s.h = w, h
	s.list.SetSize(w, h)
}

// Update forwards messages to the embedded list (navigation, filtering).
func (s Sidebar) Update(msg tea.Msg) (Sidebar, tea.Cmd) {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// View renders the bordered panel.
func (s Sidebar) View(focused bool) string {
	title := fmt.Sprintf("connections (%d)", len(s.list.Items()))
	return components.TitledBox(title, s.list.View(), s.w, s.h, focused)
}

// SelectedConfig returns the highlighted config.
func (s Sidebar) SelectedConfig() (vpn.Config, bool) {
	it, ok := s.list.SelectedItem().(item)
	if !ok {
		return vpn.Config{}, false
	}
	return vpn.Config(it), true
}

// SelectedName returns the highlighted connection name, or "".
func (s Sidebar) SelectedName() string {
	if it, ok := s.list.SelectedItem().(item); ok {
		return it.Name
	}
	return ""
}

// Filtering reports whether the list is currently capturing filter input.
func (s Sidebar) Filtering() bool {
	return s.list.FilterState() == list.Filtering
}
