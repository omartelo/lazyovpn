package dialog

import (
	"strings"
	"testing"
)

// The menu renders each action as two columns and shows the connection name in
// the title. With a single-char key the label sits a fixed gap past it; this
// guards the key→label rendering (and the title) from silently breaking.
func TestMenuViewRendersColumns(t *testing.T) {
	m := NewMenu()
	m.Open("home-vpn")
	out := m.View()

	for _, want := range []string{"home-vpn", "r", "rename connection", "f", "forget saved credentials", "d", "delete connection", "esc: close"} {
		if !strings.Contains(out, want) {
			t.Errorf("menu view missing %q\n%s", want, out)
		}
	}
	// Key and label are separated (columned), not glued as "f:forget".
	if strings.Contains(out, "f:forget") {
		t.Errorf("key and label not columned:\n%s", out)
	}
}
