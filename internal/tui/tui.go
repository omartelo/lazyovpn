// Package tui is the lazyovpn bubbletea interface: a connection sidebar + a log pane.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/omartelo/lazyovpn/internal/vpn"
)

const (
	sidebarWidth = 42
	// connectedMarker is the openvpn log line that signals the tunnel is up.
	connectedMarker = "Initialization Sequence Completed"
)

var (
	helpStyle = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	nameStyle = lipgloss.NewStyle().Faint(true)
)

// helpKeys is the keybinding footer, lazydocker style.
const helpKeys = "↑/↓ j/k: navigate · /: filter · enter: connect · d: disconnect · q: quit"

// titledBox draws a rounded box with the title inlined into the top border,
// lazydocker style. lipgloss has no native border title, so we build it by hand.
// content is padded/truncated to exactly innerW x innerH.
func titledBox(title, content string, innerW, innerH int, focused bool) string {
	borderC, titleC := lipgloss.Color("240"), lipgloss.Color("252")
	if focused {
		borderC, titleC = lipgloss.Color("205"), lipgloss.Color("205")
	}
	bs := lipgloss.NewStyle().Foreground(borderC)
	ts := lipgloss.NewStyle().Foreground(titleC).Bold(true)

	span := innerW + 2 // chars between corners (1 space of padding each side)

	label := " " + title + " "
	fill := span - 1 - lipgloss.Width(label) // -1 for the leading dash
	var top string
	if fill < 0 {
		top = bs.Render("╭" + strings.Repeat("─", span) + "╮") // too narrow for the title
	} else {
		top = bs.Render("╭─") + ts.Render(label) + bs.Render(strings.Repeat("─", fill)+"╮")
	}
	bottom := bs.Render("╰" + strings.Repeat("─", span) + "╯")
	side := bs.Render("│")

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	lines = lines[:innerH]
	cell := lipgloss.NewStyle().Width(innerW)

	var b strings.Builder
	b.WriteString(top + "\n")
	for _, ln := range lines {
		b.WriteString(side + " " + cell.Render(ln) + " " + side + "\n")
	}
	b.WriteString(bottom)
	return b.String()
}

// connState is the lifecycle of the active connection.
type connState int

const (
	stateIdle connState = iota
	stateConnecting
	stateConnected
	stateDisconnected
	stateError
)

// badge renders the colored status indicator for a state.
func (s connState) badge() string {
	label, color := "idle", "240"
	switch s {
	case stateConnecting:
		label, color = "connecting...", "214"
	case stateConnected:
		label, color = "connected", "42"
	case stateDisconnected:
		label, color = "disconnected", "240"
	case stateError:
		label, color = "error", "196"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render("● " + label)
}

// logMsg carries one output line plus its source channel (to drop stale logs).
type logMsg struct {
	ch   <-chan string
	line string
}
type logClosedMsg struct{ ch <-chan string }

func waitForLog(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return logClosedMsg{ch}
		}
		return logMsg{ch, line}
	}
}

// item adapts vpn.Config to the bubbles list.
type item vpn.Config

func (i item) Title() string       { return i.Name }
func (i item) Description() string { return i.Path }
func (i item) FilterValue() string { return i.Name }

type model struct {
	list       list.Model
	vp         viewport.Model
	mgr        *vpn.Manager
	logCh      <-chan string               // stream of the active connection
	activeName string                      // connection feeding logCh
	buffers    map[string]*strings.Builder // accumulated output per connection
	shownName  string                      // connection shown in the viewport
	state      connState
	errMsg     string
	w, h       int
	ready      bool
}

// New builds the initial model from the already-discovered configs.
func New(configs []vpn.Config, mgr *vpn.Manager) model {
	items := make([]list.Item, len(configs))
	for i, c := range configs {
		items[i] = item(c)
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false // narrow sidebar: name only

	l := list.New(items, d, 0, 0)
	l.SetShowTitle(false)     // pane title lives in the border now
	l.SetShowStatusBar(false) // count goes in the border title
	l.SetShowHelp(false)

	return model{
		list:    l,
		mgr:     mgr,
		buffers: map[string]*strings.Builder{},
		state:   stateIdle,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout(msg.Width, msg.Height)
		return m, nil

	case logMsg:
		if msg.ch != m.logCh {
			return m, nil // log from an old connection
		}
		if strings.Contains(msg.line, connectedMarker) {
			m.state = stateConnected
		}
		if b := m.buffers[m.activeName]; b != nil {
			b.WriteString(msg.line + "\n")
			if m.shownName == m.activeName {
				m.vp.SetContent(b.String())
				m.vp.GotoBottom()
			}
		}
		return m, waitForLog(m.logCh)

	case logClosedMsg:
		if msg.ch == m.logCh {
			if b := m.buffers[m.activeName]; b != nil {
				b.WriteString("\n[process exited]\n")
			}
			if m.shownName == m.activeName {
				m.showBuffer(m.activeName)
			}
			m.logCh = nil
			m.activeName = ""
			m.state = stateDisconnected
		}
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break // let the filter consume the keys
		}
		switch msg.String() {
		case "q", "ctrl+c":
			_ = m.mgr.Disconnect()
			return m, tea.Quit
		case "enter":
			return m.connectSelected()
		case "d":
			_ = m.mgr.Disconnect()
			m.logCh = nil
			m.activeName = ""
			m.state = stateDisconnected
			return m, nil
		}
	}

	// Navigation (j/k/arrows come from the list). On selection change, show that connection's output.
	prev := m.shownName
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if sel := m.selectedName(); sel != prev {
		m.showBuffer(sel)
	}
	return m, cmd
}

func (m model) connectSelected() (tea.Model, tea.Cmd) {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil
	}
	ch, err := m.mgr.Connect(vpn.Config(sel))
	if err != nil {
		m.state = stateError
		m.errMsg = err.Error()
		return m, nil
	}
	m.logCh = ch
	m.activeName = sel.Name
	m.buffers[sel.Name] = &strings.Builder{} // reconnecting clears the old log
	m.shownName = sel.Name
	m.state = stateConnecting
	m.errMsg = ""
	m.vp.SetContent("")
	m.vp.GotoTop()
	return m, waitForLog(ch)
}

// showBuffer renders connection `name`'s output into the viewport (placeholder if empty).
func (m *model) showBuffer(name string) {
	m.shownName = name
	if b, ok := m.buffers[name]; ok {
		m.vp.SetContent(b.String())
	} else {
		m.vp.SetContent("(no output — press enter to connect)")
	}
	m.vp.GotoBottom()
}

func (m model) selectedName() string {
	if it, ok := m.list.SelectedItem().(item); ok {
		return it.Name
	}
	return ""
}

// dims returns the inner content size of each pane (excluding border + padding).
func (m model) dims() (sideW, sideH, outW, outH int) {
	bodyH := m.h - 2 // status + help rows
	sideW = sidebarWidth - 4
	outW = m.w - sidebarWidth - 4
	sideH = bodyH - 2 // top + bottom border
	outH = bodyH - 2
	for _, p := range []*int{&sideW, &outW, &sideH, &outH} {
		if *p < 1 {
			*p = 1
		}
	}
	return
}

// layout recomputes both pane sizes. Reserves 2 rows: status + help.
func (m *model) layout(w, h int) {
	m.w, m.h = w, h
	sideW, sideH, outW, outH := m.dims()
	m.list.SetSize(sideW, sideH)
	if !m.ready {
		m.vp = viewport.New(outW, outH)
		m.ready = true
		m.showBuffer(m.selectedName()) // initial state
	} else {
		m.vp.Width, m.vp.Height = outW, outH
	}
}

func (m model) View() string {
	if !m.ready {
		return "loading..."
	}
	sideW, sideH, outW, outH := m.dims()

	leftTitle := fmt.Sprintf("connections (%d)", len(m.list.Items()))
	rightTitle := "terminal"
	if m.shownName != "" {
		rightTitle = "terminal — " + m.shownName
	}

	left := titledBox(leftTitle, m.list.View(), sideW, sideH, true) // sidebar is the focused pane
	right := titledBox(rightTitle, m.vp.View(), outW, outH, false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return body + "\n" + m.statusLine() + "\n" + helpStyle.Render(helpKeys)
}

func (m model) statusLine() string {
	line := " " + m.state.badge()
	if m.state == stateError && m.errMsg != "" {
		line += nameStyle.Render(": " + m.errMsg)
	} else if m.activeName != "" {
		line += "  " + nameStyle.Render(m.activeName)
	}
	return line
}
