package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/omartelo/lazyovpn/internal/tui"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

var version = "0.1.0-dev"

func main() {
	root := &cobra.Command{
		Use:     "lazyovpn",
		Short:   "TUI for managing OpenVPN connections",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			configs, err := vpn.Discover()
			if err != nil {
				return err
			}
			if len(configs) == 0 {
				return fmt.Errorf("no configs found in /etc/openvpn or ~/.config/lazyovpn")
			}

			mgr := vpn.NewManager()
			// v2: alt screen is declarative — set in the model's View(), not here.
			p := tea.NewProgram(tui.New(configs, mgr))
			_, err = p.Run()
			return err
		},
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
