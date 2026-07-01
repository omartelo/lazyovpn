package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/omartelo/lazyovpn/internal/applog"
)

func newLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log",
		Short: "Open the most recent application log in $EDITOR (falls back to vi)",
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := applog.Latest()
			if err != nil {
				return err
			}
			return openInEditor(path)
		},
	}
}

// openInEditor opens path in $EDITOR (falling back to vi), inheriting the
// terminal so an interactive editor works. $EDITOR may carry flags (e.g.
// "code -w", "nvim -R"), so it is split into command + args.
func openInEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open editor %q: %w", editor, err)
	}
	return nil
}
