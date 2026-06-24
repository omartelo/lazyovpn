// Package vpn discovers OpenVPN configs and manages connections via a privileged process.
package vpn

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// logBufferLines is the buffered capacity of a connection's log channel — enough
// to absorb openvpn's startup burst without blocking the scanner goroutine.
const logBufferLines = 100

// Manager holds the active connection. Only one at a time.
//
// ponytail: single global connection. Make it map[name]*exec.Cmd if multi-connection matters.
type Manager struct {
	mu     sync.Mutex
	active *exec.Cmd
	done   chan struct{} // closed when the current connection is torn down
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

// NeedsAuth reports whether the config asks for interactive username/password —
// a bare `auth-user-pass` directive with no credentials file. With a file
// argument the credentials are already on disk, so no prompt is needed.
func NeedsAuth(c Config) (bool, error) {
	f, err := os.Open(c.Path)
	if err != nil {
		return false, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] == "auth-user-pass" && len(fields) == 1 {
			return true, nil // bare directive: openvpn would prompt on a tty we don't have
		}
	}
	if err := sc.Err(); err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}
	return false, nil
}

// writeCredsFile writes username/password to a private, RAM-backed temp file for
// openvpn's --auth-user-pass. The caller removes it once the process is done.
//
// ponytail: the file lives for the whole connection on tmpfs (0600, removed on
// exit). Tighten to a FIFO / delete-after-read if the same-user read window matters.
func writeCredsFile(username, password string) (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR") // tmpfs, user-owned (/run/user/UID)
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "lazyovpn-auth-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	if _, err := fmt.Fprintf(f, "%s\n%s\n", username, password); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Connect starts openvpn via pkexec (prompts the user for root) and returns a
// channel of stdout+stderr lines. The channel closes when openvpn exits.
// Any previous connection is killed first.
//
// When username/password are non-empty they are written to a temporary
// credentials file passed via --auth-user-pass; the file is removed when the
// connection ends and is never persisted to durable storage.
//
// ponytail: pkexec hard-coded. Fall back to sudo if pkexec is gone — add when someone needs it.
func (m *Manager) Connect(c Config, username, password string) (<-chan string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop()

	args := []string{"openvpn", "--config", c.Path}
	var credsPath string
	if username != "" || password != "" {
		p, err := writeCredsFile(username, password)
		if err != nil {
			return nil, fmt.Errorf("write credentials: %w", err)
		}
		credsPath = p
		args = append(args, "--auth-user-pass", credsPath)
	}

	cmd := exec.Command("pkexec", args...)
	pr, pw, err := os.Pipe()
	if err != nil {
		if credsPath != "" {
			os.Remove(credsPath)
		}
		return nil, fmt.Errorf("create pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		if credsPath != "" {
			os.Remove(credsPath)
		}
		return nil, fmt.Errorf("start openvpn: %w", err)
	}
	pw.Close() // the child kept its own copy of the fd

	logs := make(chan string, logBufferLines)
	done := make(chan struct{})
	m.active = cmd
	m.done = done

	// ponytail: switching connections tears down the previous one async — brief overlap
	// of two openvpn processes. Add synchronous teardown if they fight over routes.
	go func() {
		defer close(logs)
		if credsPath != "" {
			defer os.Remove(credsPath) // creds gone when the connection ends
		}
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
	m.done = nil
}
