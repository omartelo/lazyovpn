// Package cmd wires the cobra command tree for lazyovpn.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute builds the command tree and runs it. version is injected from main
// (ldflags -X main.version at release; the dev fallback otherwise).
func Execute(version string) {
	if err := newRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "lazyovpn",
		Short:   "The lazier way to manage openvpn connections",
		Version: version,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		// We print errors ourselves in Execute; keep cobra from double-printing
		// or dumping usage on a runtime failure.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runApp()
		},
	}
	root.AddCommand(newDoctorCmd())
	return root
}
