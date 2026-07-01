package dialog

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

// menuWidth is the inner width of the action menu; height auto-fits.
const menuWidth = 44

// menuKey styles the keybind column so the hotkeys stand out from their labels.
var menuKey = lipgloss.NewStyle().Foreground(common.Accent)

// menuItems are the menu's actions, rendered as two aligned columns: keybind,
// then label. The UI routes each key (see updateMenu) — this is display only.
// A new per-connection action is one more row here plus a case in updateMenu.
var menuItems = []struct{ key, label string }{
	{"r", "rename connection"},
	{"f", "forget saved credentials"},
}

// Menu is the per-connection action menu (opened with x). It lists the actions
// available for the selected connection; the UI routes each action key (see
// updateMenu). Stateless beyond the connection name, like Confirm.
type Menu struct {
	connName string
}

// NewMenu builds the action menu.
func NewMenu() Menu { return Menu{} }

// Open points the menu at the connection it acts on.
func (m *Menu) Open(connName string) { m.connName = connName }

// View renders the menu box. The UI centers it over the main view.
func (m Menu) View() string {
	keyW := 0
	for _, it := range menuItems {
		if len(it.key) > keyW {
			keyW = len(it.key)
		}
	}

	var b strings.Builder
	for _, it := range menuItems {
		key := menuKey.Render(fmt.Sprintf("%-*s", keyW, it.key))
		fmt.Fprintf(&b, "%s   %s\n", key, it.label)
	}
	b.WriteString("\n" + common.Hint.Render("esc: close"))

	return common.Popup{Title: "menu — " + m.connName, Width: menuWidth}.Render(b.String())
}
