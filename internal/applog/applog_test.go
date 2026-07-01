package applog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogDir(t *testing.T) {
	t.Run("honors XDG_STATE_HOME", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "/xdg/state")
		got, err := LogDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := "/xdg/state/lazyovpn/logs"; got != want {
			t.Fatalf("LogDir = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.local/state", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", "/home/tester")
		got, err := LogDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := "/home/tester/.local/state/lazyovpn/logs"; got != want {
			t.Fatalf("LogDir = %q, want %q", got, want)
		}
	})
}

// touch creates an empty file named n in dir, failing the test on error.
func touch(t *testing.T, dir, n string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	// Timestamped run logs, oldest first. keepLogs of these should survive.
	names := []string{
		"lazyovpn-20240101-000001.log",
		"lazyovpn-20240101-000002.log",
		"lazyovpn-20240101-000003.log",
		"lazyovpn-20240101-000004.log",
		"lazyovpn-20240101-000005.log",
	}
	for _, n := range names {
		touch(t, dir, n)
	}
	// A stray non-run file must never be pruned or counted.
	touch(t, dir, "notes.txt")

	prune(dir, 3)

	got, err := logFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := names[len(names)-3:] // newest 3
	if len(got) != len(want) {
		t.Fatalf("after prune got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after prune got %v, want %v", got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("prune removed a non-run file: %v", err)
	}
}

func TestPruneNoopUnderKeep(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "lazyovpn-20240101-000001.log")
	touch(t, dir, "lazyovpn-20240101-000002.log")

	prune(dir, 10) // fewer files than keep — nothing removed

	got, err := logFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("prune removed files under keep threshold: got %v", got)
	}
}

func TestLatest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir) // LogDir -> dir/lazyovpn/logs (isolated per test)

	logDir, err := LogDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Latest(); err == nil {
		t.Fatal("Latest with no files: want error, got nil")
	}

	touch(t, logDir, "lazyovpn-20240101-000001.log")
	touch(t, logDir, "lazyovpn-20240101-000009.log") // newest by name
	touch(t, logDir, "lazyovpn-20240101-000005.log")

	got, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(logDir, "lazyovpn-20240101-000009.log"); got != want {
		t.Fatalf("Latest = %q, want %q", got, want)
	}
}
