package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/omartelo/lazyovpn/internal/files"
	"github.com/omartelo/lazyovpn/internal/ui/model"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

var version = "0.1.0-dev"

// dep is an external program lazyovpn relies on. bins is an any-of list (the
// file chooser has two acceptable backends); the dep counts as found if any of
// them is on PATH.
type dep struct {
	label    string
	bins     []string
	required bool
	hint     string
}

// runDoctor probes each dependency with look (exec.LookPath in production,
// stubbed in tests), returning a human-readable report and whether every
// required dependency is present.
func runDoctor(look func(string) (string, error)) (string, bool) {
	deps := []dep{
		{"openvpn", []string{"openvpn"}, true, "the OpenVPN client (your distro's openvpn package)"},
		{"pkexec", []string{"pkexec"}, true, "polkit's pkexec, used to run openvpn as root"},
		{"file chooser", files.ChooserBins(), false, "zenity or kdialog, for importing configs in the TUI"},
	}

	var b strings.Builder
	ok := true
	for _, d := range deps {
		found := ""
		for _, bin := range d.bins {
			if path, err := look(bin); err == nil {
				found = path
				break
			}
		}
		switch {
		case found != "":
			fmt.Fprintf(&b, "  ✓ %s — %s\n", d.label, found)
		case d.required:
			ok = false
			fmt.Fprintf(&b, "  ✗ %s — missing (required): %s\n", d.label, d.hint)
		default:
			fmt.Fprintf(&b, "  ⚠ %s — missing (optional): %s\n", d.label, d.hint)
		}
	}
	return b.String(), ok
}

func main() {
	root := &cobra.Command{
		Use:     "lazyovpn",
		Short:   "TUI for managing OpenVPN connections",
		Version: version,
		// We print errors ourselves below; keep cobra from double-printing or
		// dumping usage on a runtime failure.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No configs is fine — the user can import one in the TUI with "a".
			configs := vpn.Discover()

			mgr := vpn.NewManager()
			// v2: alt screen is declarative — set in the model's View(), not here.
			p := tea.NewProgram(model.New(configs, mgr))
			_, err := p.Run()
			return err
		},
	}

	root.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check that the external programs lazyovpn needs are installed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, ok := runDoctor(exec.LookPath)
			fmt.Fprint(cmd.OutOrStdout(), report)
			if !ok {
				return fmt.Errorf("missing required dependencies")
			}
			return nil
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
