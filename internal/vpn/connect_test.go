package vpn

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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
	case "sleep":
		// Stay alive until Kill'd (no goroutines parked → real sleep, not select{}).
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

// connectedMarker mirrors the UI's tunnel-up sentinel; the helper emits it so the
// stream test reads a realistic openvpn line. Kept here to avoid importing model.
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

// The whole reason creds go to tmpfs: written for the process, gone when it ends.
// This proves the file is created, passed to openvpn with the right content, and
// removed once the connection closes (hard invariant: no plaintext left behind).
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

// swap replaces execCommand for the duration of a test and restores it after.
func swap(t *testing.T, fn func(string, ...string) *exec.Cmd) {
	t.Helper()
	old := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = old })
}
