// Package vpn discovers OpenVPN configs and manages connections via a privileged process.
package vpn

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/omartelo/lazyovpn/internal/files"
)

// configDirs are the system locations scanned for .ovpn/.conf files, in order.
// The user's own connections dir (ConnectionsDir) is scanned on top of these.
var configDirs = []string{
	"/etc/openvpn/client",
	"/etc/openvpn",
}

// Config is an OpenVPN configuration file found on disk.
type Config struct {
	Name string // base name without extension
	Path string
}

// isConfigExt reports whether ext is an OpenVPN config extension (.ovpn/.conf).
func isConfigExt(ext string) bool {
	return ext == ".ovpn" || ext == ".conf"
}

// logBufferLines is the buffered capacity of a connection's log channel — enough
// to absorb openvpn's startup burst without blocking the scanner goroutine.
const logBufferLines = 100

// privateFileMode is the mode for files that may hold secrets — inline keys or
// credentials: owner read/write only.
const privateFileMode os.FileMode = 0o600

// execCommand builds the privileged openvpn command. A package var so tests can
// swap pkexec/openvpn for a helper subprocess (see connect_test.go).
var execCommand = exec.Command

// Manager holds the active connection.
//
// Single global connection. Make it map[name]*exec.Cmd if multi-connection matters.
type Manager struct {
	mu       sync.Mutex
	active   *exec.Cmd
	done     chan struct{} // closed when the current connection is torn down
	mgmtSock string        // openvpn's management socket — how we tell it to quit
}

func NewManager() *Manager { return &Manager{} }

// ConnectionsDir is where lazyovpn stores user-added configs:
// ~/.config/lazyovpn/connections.
func ConnectionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "lazyovpn", "connections"), nil
}

// Discover scans the system directories plus ConnectionsDir for configs.
// Discovery is best-effort: unreadable or missing directories are skipped, so it
// never fails.
func Discover() []Config {
	dirs := configDirs
	if dir, err := ConnectionsDir(); err == nil {
		dirs = append(dirs, dir)
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
			if !isConfigExt(ext) {
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
	return configs
}

// ImportConfig copies an .ovpn/.conf file into ConnectionsDir and returns the
// resulting Config. It rejects anything that is not an OpenVPN config and
// overwrites an existing file of the same name (re-import updates in place).
func ImportConfig(srcPath string) (Config, error) {
	ext := filepath.Ext(srcPath)
	if !isConfigExt(ext) {
		return Config{}, fmt.Errorf("not an OpenVPN config (.ovpn/.conf): %s", srcPath)
	}
	dir, err := ConnectionsDir()
	if err != nil {
		return Config{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Config{}, fmt.Errorf("create connections dir: %w", err)
	}
	base := filepath.Base(srcPath)
	dst := filepath.Join(dir, base)
	if err := files.Copy(srcPath, dst, privateFileMode); err != nil {
		return Config{}, fmt.Errorf("import config: %w", err)
	}
	return Config{Name: strings.TrimSuffix(base, ext), Path: dst}, nil
}

// eachDirective calls match for every active directive line in the config —
// blank lines and #/; comments skipped — stopping at the first line that
// matches. It reports whether any line matched. fields is always non-empty.
func eachDirective(c Config, match func(fields []string) bool) (bool, error) {
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
		if match(strings.Fields(line)) {
			return true, nil
		}
	}
	if err := sc.Err(); err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}
	return false, nil
}

// NeedsAuth reports whether the config asks for interactive username/password —
// a bare `auth-user-pass` directive with no credentials file. With a file
// argument the credentials are already on disk, so no prompt is needed.
func NeedsAuth(c Config) (bool, error) {
	return eachDirective(c, func(f []string) bool {
		// bare directive: openvpn would prompt on a tty we don't have
		return f[0] == "auth-user-pass" && len(f) == 1
	})
}

// HasDaemon reports whether the config forks into the background (a `daemon`
// directive). openvpn's foreground process then exits right after setup, so
// lazyovpn can't track the tunnel — auto-reconnect must stay off for such a
// config or it would spawn a duplicate process on top of the live daemon.
func HasDaemon(c Config) (bool, error) {
	return eachDirective(c, func(f []string) bool { return f[0] == "daemon" })
}

// writeCredsFile writes username/password to a private, RAM-backed temp file for
// openvpn's --auth-user-pass. The caller removes it once the process is done.
//
// The file lives for the whole connection on tmpfs (0600, removed on exit).
// Tighten to a FIFO / delete-after-read if the same-user read window matters.
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
	if err := f.Chmod(privateFileMode); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	if _, err := fmt.Fprintf(f, "%s\n%s\n", username, password); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// openPTY allocates a pseudo-terminal and returns its master (read) and slave
// (write) ends. openvpn's stdout/stderr go to the slave so the child sees a tty
// and libc line-buffers its log. Over a plain pipe openvpn block-buffers (4KB);
// at low verb the tunnel-up line "Initialization Sequence Completed" — the last
// line, with nothing after it to fill the buffer — never reaches us until the
// process exits, so the UI hangs on "connecting…" for an already-up tunnel.
// Linux-only, like the rest of the tool (file chooser, etc.).
func openPTY() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open ptmx: %w", err)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("pty number: %w", err)
	}
	slave, err = os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("open pts %d: %w", n, err)
	}
	return master, slave, nil
}

// currentUsername is the OS user the TUI runs as, for openvpn's
// --management-client-user — so the root openvpn lets this (unprivileged) user
// connect to its management socket. Empty if it can't be determined.
func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// mgmtSocketPath reserves a unique, not-yet-existing path in the user's runtime
// dir for openvpn's management unix socket. CreateTemp gives a collision-free
// name; we remove the file so openvpn (root) can bind a socket there.
func mgmtSocketPath() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR") // tmpfs, user-owned (/run/user/UID)
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "lazyovpn-mgmt-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

// signalQuit asks openvpn to terminate itself over its management socket. The
// TUI is unprivileged and openvpn runs as root (pkexec), so a direct kill(2) is
// EPERM — this is the real teardown path. Best-effort and bounded: a dial error
// just means openvpn is already gone or never opened the socket (the Kill
// fallback in stop covers a non-pkexec/test process).
//
// Runs on the UI goroutine via stop; the 1s timeouts bound any hang on a stale
// socket. Move to a tea.Cmd if that ever bites.
func signalQuit(sock string) {
	c, err := net.DialTimeout("unix", sock, time.Second)
	if err != nil {
		return
	}
	defer c.Close()
	// Line-based protocol: openvpn sends a banner on connect, but acts on commands
	// without us reading it. "signal SIGTERM" makes it exit its event loop.
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte("signal SIGTERM\n")); err != nil {
		return
	}
	// Block until openvpn acts: it replies, then exits, closing the socket (read →
	// EOF). Draining makes teardown deterministic — stop returns only once openvpn
	// is actually gone — instead of racing our own Close against the caller's next
	// step (a switch's new process, or the program quitting). Bounded by the
	// deadline so a wedged openvpn can't hang the UI goroutine.
	_, _ = io.Copy(io.Discard, c)
}

// Connect starts openvpn via pkexec (prompts the user for root) and returns a
// channel of stdout+stderr lines. The channel closes when openvpn exits.
// Any previous connection is killed first.
//
// When username/password are non-empty they are written to a temporary
// credentials file passed via --auth-user-pass; the file is removed when the
// connection ends and is never persisted to durable storage.
//
// pkexec hard-coded. Fall back to sudo if pkexec is gone — add when someone needs it.
func (m *Manager) Connect(c Config, username, password string) (<-chan string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop()

	args := []string{"openvpn", "--config", c.Path}

	// Management socket: the only way the unprivileged TUI can stop a root
	// openvpn (signalQuit) — kill(2) on it is EPERM. Best-effort: if we can't
	// resolve the OS user the socket would be unreachable, so skip it and accept
	// the teardown ceiling rather than open a socket we can't talk to.
	var mgmtSock string
	if osUser := currentUsername(); osUser != "" {
		if sock, err := mgmtSocketPath(); err == nil {
			mgmtSock = sock
			args = append(args, "--management", sock, "unix", "--management-client-user", osUser)
		}
	}

	var credsPath string
	if username != "" || password != "" {
		p, err := writeCredsFile(username, password)
		if err != nil {
			return nil, fmt.Errorf("write credentials: %w", err)
		}
		credsPath = p
		args = append(args, "--auth-user-pass", credsPath)
	}

	cmd := execCommand("pkexec", args...)
	// A pty (not a plain pipe) so openvpn line-buffers — see openPTY. We read its
	// combined stdout+stderr off the master end.
	master, slave, err := openPTY()
	if err != nil {
		if credsPath != "" {
			os.Remove(credsPath)
		}
		return nil, fmt.Errorf("create pty: %w", err)
	}
	cmd.Stdout = slave
	cmd.Stderr = slave

	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		if credsPath != "" {
			os.Remove(credsPath)
		}
		return nil, fmt.Errorf("start openvpn: %w", err)
	}
	slave.Close() // the child kept its own copy of the fd

	logs := make(chan string, logBufferLines)
	done := make(chan struct{})
	m.active = cmd
	m.done = done
	m.mgmtSock = mgmtSock

	// Switching connections tears down the previous one async — brief overlap of two
	// openvpn processes. Add synchronous teardown if they fight over routes.
	go func() {
		defer close(logs)
		if credsPath != "" {
			defer os.Remove(credsPath) // creds gone when the connection ends
		}
		if mgmtSock != "" {
			defer os.Remove(mgmtSock) // openvpn unlinks it on a clean exit; tidy up otherwise
		}
		defer cmd.Wait() // reaper: runs on process exit (its own or via Kill)
		defer master.Close()
		// On child exit the pty master read returns EIO, not a clean EOF; the
		// scanner stops either way and we don't inspect sc.Err(), so it reads as a
		// normal end-of-stream (same as the old pipe's EOF).
		sc := bufio.NewScanner(master)
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
	if m.mgmtSock != "" {
		signalQuit(m.mgmtSock) // root openvpn terminates itself; kill(2) below would EPERM
	}
	_ = m.active.Process.Kill() // fallback: reaps a non-pkexec/test process
	m.active = nil
	m.done = nil
	m.mgmtSock = ""
}
