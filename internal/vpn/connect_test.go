package vpn

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// These tests exercise Manager.Connect/Disconnect/stop — the privileged process
// lifecycle — without a real pkexec/openvpn. execCommand is swapped for a
// re-exec of this test binary (the os/exec TestHelperProcess pattern); the
// helper plays "openvpn" and is driven by HELPER_MODE.

// fakeExec returns an execCommand replacement that runs TestHelperProcess in the
// given mode instead of the real pkexec command.
func fakeExec(mode string) func(string, ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_MODE="+mode,
		)
		return cmd
	}
}

// TestHelperProcess is not a real test: it is the fake "openvpn" spawned by
// fakeExec. It only acts when GO_WANT_HELPER_PROCESS=1 is set in its env.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// Args after "--" are the real command: pkexec openvpn --config <p> [--auth-user-pass <p>]
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	credPath := ""
	for i, a := range args {
		if a == "--auth-user-pass" && i+1 < len(args) {
			credPath = args[i+1]
		}
	}

	// Always report the creds-file path first (empty when none was passed) so
	// every test can assert whether Connect wrote one.
	os.Stdout.WriteString("CREDPATH " + credPath + "\n")

	// Report the management socket + client-user so tests can assert Connect
	// wired the teardown channel (signalQuit's target).
	mgmtPath, mgmtUser := "", ""
	for i, a := range args {
		switch a {
		case "--management":
			if i+1 < len(args) {
				mgmtPath = args[i+1]
			}
		case "--management-client-user":
			if i+1 < len(args) {
				mgmtUser = args[i+1]
			}
		}
	}
	os.Stdout.WriteString("MGMT " + mgmtPath + " " + mgmtUser + "\n")

	switch os.Getenv("HELPER_MODE") {
	case "echo":
		// Two lines, the second being openvpn's real tunnel-up marker.
		os.Stdout.WriteString("line one\n")
		os.Stdout.WriteString(connectedMarker + "\n")
	case "creds":
		// Echo the creds-file content so the test can verify the credentials
		// reached "openvpn" intact via --auth-user-pass.
		data, _ := os.ReadFile(credPath)
		for _, ln := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			os.Stdout.WriteString("CRED " + ln + "\n")
		}
	case "tty":
		// Prove openvpn's stdout is a tty. TCGETS succeeds only on a terminal; a
		// plain pipe would make libc block-buffer the tunnel-up marker.
		if _, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), unix.TCGETS); err == nil {
			os.Stdout.WriteString("ISATTY yes\n")
		} else {
			os.Stdout.WriteString("ISATTY no\n")
		}
	case "sleep":
		// Stay alive until Kill'd (no goroutines parked → real sleep, not select{}).
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

// connectedMarker is just a realistic openvpn log line for the stream test — the
// UI no longer scrapes it (tunnel-up comes from the management state); it remains
// only as representative output the fake "openvpn" emits.
const connectedMarker = "Initialization Sequence Completed"

// drain reads every line until the channel closes (proving the reaper closed it)
// and fails on timeout instead of hanging the whole suite.
func drain(t *testing.T, ch <-chan string) []string {
	t.Helper()
	var lines []string
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ln, ok := <-ch:
			if !ok {
				return lines
			}
			lines = append(lines, ln)
		case <-deadline:
			t.Fatal("log channel never closed — reaper did not run")
			return nil
		}
	}
}

func TestConnectStreamsThenCloses(t *testing.T) {
	swap(t, fakeExec("echo"))
	cfg := writeConfig(t, "client\nremote x 1194\n")

	ch, err := NewManager().Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var lines []string
	for _, ln := range drain(t, ch) {
		if path := strings.TrimPrefix(ln, "CREDPATH "); path != ln {
			// A no-auth connect must not write a credentials file.
			if path != "" {
				t.Errorf("no-auth connect passed --auth-user-pass %q, want none", path)
			}
			continue
		}
		if strings.HasPrefix(ln, "MGMT ") {
			continue
		}
		lines = append(lines, ln)
	}

	want := []string{"line one", connectedMarker}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

// Connect must hand openvpn a tty for stdout, not a pipe. Over a pipe libc
// block-buffers openvpn's log, so at low verb the final "Initialization Sequence
// Completed" line never reaches the UI until the process exits — the tunnel
// shows "connecting…" forever despite being up. Swapping the pty back to
// os.Pipe flips this assertion to "ISATTY no".
func TestConnectGivesChildATTY(t *testing.T) {
	swap(t, fakeExec("tty"))
	cfg := writeConfig(t, "client\n")

	ch, err := NewManager().Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	saw := false
	for _, ln := range drain(t, ch) {
		switch ln {
		case "ISATTY yes":
			saw = true
		case "ISATTY no":
			t.Error("openvpn stdout is not a tty — its log will block-buffer")
		}
	}
	if !saw {
		t.Error("helper never reported tty status")
	}
}

// The whole reason creds go to tmpfs: written for the process, gone when it ends.
// This proves the file is created, passed to openvpn with the right content, and
// removed once the connection closes — no plaintext left behind.
func TestConnectWritesCredsAndRemovesOnExit(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	swap(t, fakeExec("creds"))
	cfg := writeConfig(t, "client\nauth-user-pass\n")

	ch, err := NewManager().Connect(cfg, "alice", "s3cret")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var credPath string
	var credLines []string
	for _, ln := range drain(t, ch) {
		switch {
		case strings.HasPrefix(ln, "CREDPATH "):
			credPath = strings.TrimPrefix(ln, "CREDPATH ")
		case strings.HasPrefix(ln, "CRED "):
			credLines = append(credLines, strings.TrimPrefix(ln, "CRED "))
		}
	}

	if credPath == "" {
		t.Fatal("no --auth-user-pass file was passed to openvpn")
	}
	if got, want := strings.Join(credLines, "\n"), "alice\ns3cret"; got != want {
		t.Errorf("creds delivered = %q, want %q", got, want)
	}
	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Errorf("creds file %s still exists after connection ended (stat err: %v)", credPath, err)
	}
}

// Disconnect must kill the process and let the reaper close the channel — and do
// it with exactly one cmd.Wait (a second would panic, failing the test).
func TestConnectKillTearsDown(t *testing.T) {
	swap(t, fakeExec("sleep"))
	cfg := writeConfig(t, "client\n")

	m := NewManager()
	ch, err := m.Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := m.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	drain(t, ch) // closes only if the reaper ran after the kill
}

// Switching connections kills the previous process before starting the new one;
// the old channel must close so its goroutine and creds file are reclaimed.
func TestSecondConnectKillsFirst(t *testing.T) {
	swap(t, fakeExec("sleep"))
	cfg := writeConfig(t, "client\n")

	m := NewManager()
	ch1, err := m.Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	ch2, err := m.Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	drain(t, ch1) // the first connection must be torn down by the second
	_ = m.Disconnect()
	drain(t, ch2)
}

// If openvpn fails to start, Connect must report the error and leave no
// credentials file behind — a failed connect must not leak a password file.
func TestConnectStartErrorRemovesCreds(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	swap(t, func(string, ...string) *exec.Cmd {
		return exec.Command("/nonexistent/lazyovpn-bogus") // Start() fails
	})
	cfg := writeConfig(t, "client\nauth-user-pass\n")

	if _, err := NewManager().Connect(cfg, "alice", "s3cret"); err == nil {
		t.Fatal("Connect: want error when openvpn fails to start, got nil")
	}

	entries, err := os.ReadDir(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("creds file leaked after start failure: %v", entries)
	}
}

// Without XDG_RUNTIME_DIR there is no guaranteed tmpfs, and os.TempDir() may be
// disk-backed — Connect must refuse to write the password there (passwords
// never touch durable storage) instead of falling back.
func TestConnectRefusesCredsWithoutRuntimeDir(t *testing.T) {
	swap(t, fakeExec("echo")) // backstop: a wrongly-spawned process is fake, not pkexec
	t.Setenv("XDG_RUNTIME_DIR", "")
	cfg := writeConfig(t, "client\nauth-user-pass\n")

	if _, err := NewManager().Connect(cfg, "alice", "s3cret"); err == nil {
		t.Fatal("Connect: want error without XDG_RUNTIME_DIR, got nil")
	}
}

// A switch whose setup fails (creds file unwritable here) must leave the
// previous connection running: the old tunnel is torn down only once the new
// connection is ready to start, so a failed switch doesn't kill it for nothing.
func TestFailedConnectKeepsPrevious(t *testing.T) {
	swap(t, fakeExec("sleep"))
	cfg := writeConfig(t, "client\nauth-user-pass\n")

	m := NewManager()
	ch1, err := m.Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	// Drain the helper's two banner lines; it then stays alive until killed.
	for i := 0; i < 2; i++ {
		select {
		case <-ch1:
		case <-time.After(10 * time.Second):
			t.Fatal("first connection produced no output")
		}
	}

	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "missing")) // creds write fails
	if _, err := m.Connect(cfg, "alice", "s3cret"); err == nil {
		t.Fatal("second Connect: want setup error, got nil")
	}

	select {
	case _, ok := <-ch1:
		if !ok {
			t.Fatal("failed second Connect tore down the first connection")
		}
		t.Fatal("unexpected extra output from the first connection")
	case <-time.After(100 * time.Millisecond):
		// channel still open — the first connection survived the failed switch
	}
	_ = m.Disconnect()
	drain(t, ch1)
}

// If the credentials file cannot be written, Connect fails before spawning
// anything — no process, no error swallowed.
func TestConnectWriteCredsError(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "missing")) // CreateTemp fails
	cfg := writeConfig(t, "client\nauth-user-pass\n")

	if _, err := NewManager().Connect(cfg, "alice", "s3cret"); err == nil {
		t.Fatal("Connect: want error when the creds file cannot be written, got nil")
	}
}

// Connect must wire openvpn's management unix socket and restrict it to the
// current OS user — that socket is the only way the unprivileged TUI can stop a
// root openvpn (signalQuit). Without these flags disconnect/quit leak the
// process (a plain kill is EPERM on a pkexec'd root process).
func TestConnectPassesManagementSocket(t *testing.T) {
	swap(t, fakeExec("echo"))
	cfg := writeConfig(t, "client\n")

	ch, err := NewManager().Connect(cfg, "", "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var sock, who string
	for _, ln := range drain(t, ch) {
		if rest := strings.TrimPrefix(ln, "MGMT "); rest != ln {
			parts := strings.SplitN(rest, " ", 2)
			sock = parts[0]
			if len(parts) > 1 {
				who = parts[1]
			}
		}
	}
	if sock == "" {
		t.Error("Connect did not pass --management <socket> unix")
	}
	if want := currentUsername(); who != want {
		t.Errorf("--management-client-user = %q, want %q", who, want)
	}
}

// signalQuit must send the exact line that makes openvpn exit. This is the real
// teardown for a root openvpn the TUI can't kill directly; it must not block on
// the management banner openvpn sends first.
func TestSignalQuitSendsSIGTERM(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "mgmt.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- "ACCEPT ERR: " + err.Error()
			return
		}
		defer c.Close()
		// openvpn greets first; emit a banner to prove signalQuit doesn't wait for it.
		fmt.Fprintln(c, ">INFO:OpenVPN Management Interface Version 5")
		line, _ := bufio.NewReader(c).ReadString('\n')
		got <- strings.TrimSpace(line)
	}()

	signalQuit(sock)

	select {
	case line := <-got:
		if line != "signal SIGTERM" {
			t.Errorf("management received %q, want %q", line, "signal SIGTERM")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signalQuit sent nothing to the management socket")
	}
}

// signalQuit on a dead socket must return promptly, not hang the UI goroutine.
func TestSignalQuitNoListener(t *testing.T) {
	done := make(chan struct{})
	go func() {
		signalQuit(filepath.Join(t.TempDir(), "nope.sock")) // nothing listening
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("signalQuit blocked on a dead socket")
	}
}

// QueryState returns openvpn's current management state — the authoritative
// tunnel-up signal that replaces scraping the log for a marker a low-verb/mute
// config may never print. It must pick the latest row when history has several.
func TestQueryStateReturnsCurrent(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "mgmt.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		fmt.Fprint(c, ">INFO:OpenVPN Management Interface Version 5\r\n")
		bufio.NewReader(c).ReadString('\n') // the "state" request
		// History oldest-first, CR-LF like openvpn; CONNECTED is the current row.
		fmt.Fprint(c, "1700000000,CONNECTING,,,,\r\n")
		fmt.Fprint(c, "1700000005,CONNECTED,SUCCESS,10.8.0.2,1.2.3.4,1194,,\r\n")
		fmt.Fprint(c, "END\r\n")
	}()

	if got := QueryState(sock); got != "CONNECTED" {
		t.Errorf("QueryState = %q, want CONNECTED (latest row)", got)
	}
}

// QueryState returns "" (no panic, no hang) when nothing is listening.
func TestQueryStateNoListener(t *testing.T) {
	if got := QueryState(filepath.Join(t.TempDir(), "nope.sock")); got != "" {
		t.Errorf("QueryState on a dead socket = %q, want empty", got)
	}
}

// A latest state that is not CONNECTED is returned verbatim — QueryState must not
// report a false tunnel-up (the whole point of replacing log scraping).
func TestQueryStateNotConnected(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "mgmt.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		fmt.Fprint(c, ">INFO:OpenVPN Management Interface Version 5\r\n")
		bufio.NewReader(c).ReadString('\n') // the "state" request
		fmt.Fprint(c, "1700000000,RECONNECTING,ping-restart,,,\r\nEND\r\n")
	}()

	if got := QueryState(sock); got != "RECONNECTING" {
		t.Errorf("QueryState = %q, want RECONNECTING (not a false CONNECTED)", got)
	}
}

// swap replaces execCommand for the duration of a test and restores it after.
func swap(t *testing.T, fn func(string, ...string) *exec.Cmd) {
	t.Helper()
	old := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = old })
}
