package cmd

import (
	"io"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/applog"
	"github.com/omartelo/lazyovpn/internal/ui/model"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// runApp discovers configs and launches the Bubble Tea program. Bare
// `lazyovpn` (no subcommand) lands here.
func runApp(version string) error {
	if closer, err := applog.Setup(); err != nil {
		// Logging is non-essential — never block the app on it. But keep slog off
		// its stderr default, which would corrupt the TUI alt screen.
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	} else {
		defer closer.Close()
	}
	slog.Info("lazyovpn starting", "version", version)

	// No configs is fine — the user can import one in the TUI with "a".
	configs := vpn.Discover()
	slog.Info("configs discovered", "count", len(configs))

	mgr := vpn.NewManager()
	// Tear the tunnel down on every exit path — q, ctrl+c, or error. Without this
	// a ctrl+c leaves the root openvpn running (the TUI can't kill it directly);
	// Disconnect signals it over the management socket. No-op if nothing is up.
	defer mgr.Disconnect()
	// v2: alt screen is declarative — set in the model's View(), not here.
	p := tea.NewProgram(model.New(configs, mgr))
	_, err := p.Run()
	slog.Info("lazyovpn exiting", "err", err)
	return err
}
