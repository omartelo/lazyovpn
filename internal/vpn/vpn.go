// Package vpn discovers OpenVPN configs and manages connections via a privileged process.
package vpn

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// configDirs are the locations scanned for .ovpn/.conf files, in order.
var configDirs = []string{
	"/etc/openvpn/client",
	"/etc/openvpn",
}

// Config is an OpenVPN configuration file found on disk.
type Config struct {
	Name string // base name without extension
	Path string
}

// Manager holds the active connection. Only one at a time.
//
// ponytail: single global connection. Make it map[name]*exec.Cmd if multi-connection matters.
type Manager struct {
	mu      sync.Mutex
	active  *exec.Cmd
	current string
	done    chan struct{} // closed when the current connection is torn down
}

func NewManager() *Manager { return &Manager{} }

// Discover scans the known directories plus ~/.config/lazyovpn for configs.
func Discover() ([]Config, error) {
	dirs := configDirs
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "lazyovpn"))
	}

	var configs []Config
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // missing dir is fine
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			if ext != ".ovpn" && ext != ".conf" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			configs = append(configs, Config{
				Name: e.Name()[:len(e.Name())-len(ext)],
				Path: path,
			})
		}
	}
	return configs, nil
}

// Connect starts openvpn via pkexec (prompts the user for root) and returns a
// channel of stdout+stderr lines. The channel closes when openvpn exits.
// Any previous connection is killed first.
//
// ponytail: pkexec hard-coded. Fall back to sudo if pkexec is gone — add when someone needs it.
func (m *Manager) Connect(c Config) (<-chan string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop()

	cmd := exec.Command("pkexec", "openvpn", "--config", c.Path)
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("start openvpn: %w", err)
	}
	pw.Close() // the child kept its own copy of the fd

	logs := make(chan string, 100)
	done := make(chan struct{})
	m.active = cmd
	m.current = c.Name
	m.done = done

	// ponytail: switching connections tears down the previous one async — brief overlap
	// of two openvpn processes. Add synchronous teardown if they fight over routes.
	go func() {
		defer close(logs)
		defer cmd.Wait() // reaper: runs on process exit (its own or via Kill)
		defer pr.Close()
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			select {
			case logs <- sc.Text():
			case <-done: // TUI stopped reading (switch/quit) — don't block
				return
			}
		}
	}()
	return logs, nil
}

// Disconnect kills the active connection, if any.
func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop()
	return nil
}

// stop kills the current process. Must be called with m.mu held.
// It does not call cmd.Wait — the scanner goroutine reaps.
func (m *Manager) stop() {
	if m.active == nil {
		return
	}
	close(m.done)
	_ = m.active.Process.Kill()
	m.active = nil
	m.current = ""
	m.done = nil
}

// Current returns the name of the connected config, or "" if disconnected.
func (m *Manager) Current() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}
