package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/omartelo/lazyovpn/internal/ui/model"
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
			// No configs is fine — the user can import one in the TUI with "a".

			mgr := vpn.NewManager()
			// v2: alt screen is declarative — set in the model's View(), not here.
			p := tea.NewProgram(model.New(configs, mgr))
			_, err = p.Run()
			return err
		},
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
