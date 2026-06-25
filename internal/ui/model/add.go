// add.go is the import-connection flow. The UI drives a dialog.FilePicker to
// choose an .ovpn/.conf file in the native chooser; on confirm, vpn.ImportConfig
// copies it into the connections dir and the sidebar gains the entry. The picker
// component lives in internal/ui/dialog; the business rule (validate + import)
// lives here.
package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/ui/dialog"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// updateAdd handles input while the import picker is open. esc cancels, r picks
// again, enter imports; a FilePickedMsg records the chosen path.
func (m *UI) updateAdd(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.picker.Reset()
			m.mode = modeNormal
			return m, nil
		case "r":
			return m, m.picker.Open() // launch the chooser again
		case "enter":
			return m.addConfirm()
		}
	}
	if picked, ok := msg.(dialog.FilePickedMsg); ok {
		m.picker.Handle(picked)
	}
	return m, nil
}

// addConfirm imports the picked file and appends it to the sidebar. Stays open
// on error so the user can pick again.
func (m *UI) addConfirm() (tea.Model, tea.Cmd) {
	path := m.picker.Path()
	if path == "" {
		return m, nil // nothing picked yet
	}
	cfg, err := vpn.ImportConfig(path)
	if err != nil {
		m.picker.SetError(err.Error())
		return m, nil
	}
	m.sidebar.AddConfig(cfg)
	m.picker.Reset()
	m.mode = modeNormal
	return m, nil
}
