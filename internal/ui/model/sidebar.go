// Package model holds the root bubbletea model (ui.go) and the per-panel
// sub-models it composes. Each panel owns its own state and business rules so
// new views can be added without touching the others.
package model

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// item adapts vpn.Config to the bubbles list.
type item vpn.Config

func (i item) Title() string       { return i.Name }
func (i item) Description() string { return i.Path }
func (i item) FilterValue() string { return i.Name }

var (
	connectedStyle = lipgloss.NewStyle().Foreground(common.Connected)         // green ● marker + connected row
	itemSelected   = lipgloss.NewStyle().Foreground(common.Accent).Bold(true) // pink cursor row
	itemNormal     = lipgloss.NewStyle()
)

// connDelegate renders one connection per row: a green ● prefix and green text
// on the connected entry, the pink cursor highlight on the selected one.
type connDelegate struct {
	connectedName string
}

func (d connDelegate) Height() int                         { return 1 }
func (d connDelegate) Spacing() int                        { return 0 } // blank rows between entries (0 = none)
func (d connDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d connDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}
	connected := it.Name == d.connectedName

	marker := "  " // keep names aligned whether or not the dot is shown
	if connected {
		marker = connectedStyle.Render("●") + " "
	}

	style := itemNormal
	switch {
	case index == m.Index():
		style = itemSelected
	case connected:
		style = connectedStyle
	}

	line := lipgloss.NewStyle().MaxWidth(m.Width()).Render(marker + style.Render(it.Name))
	fmt.Fprint(w, line)
}

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

	l := list.New(items, connDelegate{}, 0, 0)
	l.SetShowTitle(false)     // pane title lives in the border now
	l.SetShowStatusBar(false) // count goes in the border title
	l.SetShowHelp(false)

	return Sidebar{list: l}
}

// SetConnected marks name as the live connection (green ● + green row), or
// clears the marker when name is "".
func (s *Sidebar) SetConnected(name string) {
	s.list.SetDelegate(connDelegate{connectedName: name})
}

// AddConfig appends a freshly imported config to the list, skipping it if a
// config with the same path is already listed.
func (s *Sidebar) AddConfig(c vpn.Config) {
	for _, li := range s.list.Items() {
		if existing, ok := li.(item); ok && existing.Path == c.Path {
			return
		}
	}
	s.list.InsertItem(len(s.list.Items()), item(c))
}

// SetSize sets the inner content size (inside the border).
func (s *Sidebar) SetSize(w, h int) {
	s.w, s.h = w, h
	s.list.SetSize(w, h)
}

// Handle forwards a message to the embedded list (navigation, filtering) and
// returns any command it emits. Imperative: it mutates the sidebar in place, so
// the caller does not reassign — see internal/ui/CLAUDE.md.
func (s *Sidebar) Handle(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

// View renders the bordered panel.
func (s Sidebar) View(focused bool) string {
	title := fmt.Sprintf("connections (%d)", len(s.list.Items()))
	return common.TitledBox(title, s.list.View(), s.w, s.h, focused)
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
