// Package utils holds shared bubbletea plumbing: the vpn log stream command and messages.
package utils

import tea "charm.land/bubbletea/v2"

// LogMsg carries one output line plus its source channel (to drop stale logs).
type LogMsg struct {
	Ch   <-chan string
	Line string
}

// LogClosedMsg signals the source channel closed (process exited).
type LogClosedMsg struct{ Ch <-chan string }

// WaitForLog blocks on the channel and emits a LogMsg, or LogClosedMsg on close.
// Re-issue it after each LogMsg to pump the next line.
func WaitForLog(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return LogClosedMsg{ch}
		}
		return LogMsg{ch, line}
	}
}
