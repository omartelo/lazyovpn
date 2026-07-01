// rename.go is the rename-connection flow, reached with "r" in the action menu.
// The UI drives a dialog.Rename prompt; on confirm, vpn.RenameConfig migrates any
// saved credentials and renames the file, and the sidebar/log pick up the new
// name. The prompt lives in internal/ui/dialog; the business rule (migrate +
// rename, and the in-use guard) lives here.
package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/vpn"
)

// openRename swaps the action menu for the rename prompt, prefilled with the
// selected connection's current name. A no-op (back to normal) if nothing is
// selected.
func (m *UI) openRename() (tea.Model, tea.Cmd) {
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok {
		m.mode = modeNormal
		return m, nil
	}
	m.mode = modeRename
	return m, m.rename.Open(cfg.Name)
}

// updateRename handles input while the rename prompt is open. esc cancels, enter
// runs the rename; everything else edits the name field.
func (m *UI) updateRename(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.rename.Reset()
			m.mode = modeNormal
			return m, nil
		case "enter":
			return m.renameConfirm()
		}
	}
	return m, m.rename.Handle(msg)
}

// renameConfirm applies the rename. It refuses a connection that is in use
// (renaming would desync the live/reconnect state, which is keyed by name) and
// stays open showing any error so the user can fix the name and retry.
func (m *UI) renameConfirm() (tea.Model, tea.Cmd) {
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok {
		m.rename.Reset()
		m.mode = modeNormal
		return m, nil
	}
	if m.inUse(cfg) {
		m.rename.SetError("disconnect before renaming this connection")
		return m, nil
	}
	newCfg, err := vpn.RenameConfig(cfg, m.rename.Value())
	if err != nil {
		m.rename.SetError(err.Error())
		return m, nil
	}
	m.sidebar.RenameConfig(cfg, newCfg)
	m.log.RenameBuffer(cfg.Name, newCfg.Name) // keep stored output under the new name
	// The renamed config is the selected one, so refresh the pane to its new name.
	m.log.ShowBuffer(newCfg.Name)
	m.rename.Reset()
	m.mode = modeNormal
	return m, nil
}

// inUse reports whether cfg is the live tunnel or a connection with a pending
// auto-reconnect — renaming it would leave name-keyed state (shared.connected,
// the log's active name, the reconnect config) pointing at the old name.
func (m *UI) inUse(cfg vpn.Config) bool {
	if m.logCh != nil && cfg.Name == m.log.ActiveName() {
		return true // live tunnel
	}
	if m.reArmed && cfg.Path == m.reCfg.Path {
		return true // waiting to auto-reconnect
	}
	return false
}
