package cmd

import (
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/ui/model"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// runApp discovers configs and launches the Bubble Tea program. Bare
// `lazyovpn` (no subcommand) lands here.
func runApp() error {
	// No configs is fine — the user can import one in the TUI with "a".
	configs := vpn.Discover()

	mgr := vpn.NewManager()
	// v2: alt screen is declarative — set in the model's View(), not here.
	p := tea.NewProgram(model.New(configs, mgr))
	_, err := p.Run()
	return err
}
