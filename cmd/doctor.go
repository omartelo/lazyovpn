package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/omartelo/lazyovpn/internal/files"
)

// dep is an external program lazyovpn relies on. bins is an any-of list (the
// file chooser has two acceptable backends); the dep counts as found if any of
// them is on PATH.
type dep struct {
	label    string
	bins     []string
	required bool
	hint     string
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
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
	}
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
