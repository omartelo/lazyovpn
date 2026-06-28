package vpn

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.ovpn")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return Config{Name: "c", Path: path}
}

func TestImportConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(t.TempDir(), "myvpn.ovpn")
	if err := os.WriteFile(src, []byte("client\nremote x 1194\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ImportConfig(src)
	if err != nil {
		t.Fatalf("ImportConfig error: %v", err)
	}

	want := filepath.Join(home, ".config", "lazyovpn", "connections", "myvpn.ovpn")
	if cfg.Path != want {
		t.Errorf("Path = %q, want %q", cfg.Path, want)
	}
	if cfg.Name != "myvpn" {
		t.Errorf("Name = %q, want myvpn", cfg.Name)
	}

	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "client\nremote x 1194\n" {
		t.Errorf("copied content = %q", got)
	}
	if info, _ := os.Stat(cfg.Path); info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestImportConfigRejectsNonConfig(t *testing.T) {
	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportConfig(src); err == nil {
		t.Error("ImportConfig(.txt): want error, got nil")
	}
}

func TestNeedsAuth(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"bare directive", "client\nauth-user-pass\ndev tun\n", true},
		{"directive with file", "client\nauth-user-pass creds.txt\n", false},
		{"no directive", "client\ndev tun\nremote vpn.example 1194\n", false},
		{"commented out", "client\n# auth-user-pass\n; auth-user-pass\n", false},
		{"indented and padded", "client\n   auth-user-pass  \n", true},
		{"substring is not a match", "client\nauth-user-pass-optional\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NeedsAuth(writeConfig(t, tt.content))
			if err != nil {
				t.Fatalf("NeedsAuth error: %v", err)
			}
			if got != tt.want {
				t.Errorf("NeedsAuth = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsAuthMissingFile(t *testing.T) {
	if _, err := NeedsAuth(Config{Path: "/nonexistent/x.ovpn"}); err == nil {
		t.Error("NeedsAuth on missing file: want error, got nil")
	}
}

func TestHasDaemon(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"bare directive", "client\ndaemon\ndev tun\n", true},
		{"with progname", "client\ndaemon openvpn-client\n", true},
		{"indented and padded", "client\n   daemon  \n", true},
		{"no directive", "client\ndev tun\nremote vpn.example 1194\n", false},
		{"commented out", "client\n# daemon\n; daemon\n", false},
		{"substring is not a match", "client\ndaemonize\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HasDaemon(writeConfig(t, tt.content))
			if err != nil {
				t.Fatalf("HasDaemon error: %v", err)
			}
			if got != tt.want {
				t.Errorf("HasDaemon = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasDaemonMissingFile(t *testing.T) {
	if _, err := HasDaemon(Config{Path: "/nonexistent/x.ovpn"}); err == nil {
		t.Error("HasDaemon on missing file: want error, got nil")
	}
}

func TestDiscover(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sys1 := t.TempDir()
	sys2 := t.TempDir()
	conns := filepath.Join(home, ".config", "lazyovpn", "connections")
	if err := os.MkdirAll(conns, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("client\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(sys1, "alpha.ovpn")
	write(sys1, "notes.txt") // wrong extension — skipped
	if err := os.Mkdir(filepath.Join(sys1, "sub"), 0o755); err != nil {
		t.Fatal(err) // a directory — skipped
	}
	write(sys2, "beta.conf")
	write(conns, "gamma.ovpn")

	// sys1 listed twice exercises the dedup guard; the missing dir is skipped.
	orig := configDirs
	t.Cleanup(func() { configDirs = orig })
	configDirs = []string{sys1, sys1, sys2, "/does/not/exist"}

	got := Discover()

	names := map[string]int{}
	for _, c := range got {
		names[c.Name]++
	}
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if names[want] != 1 {
			t.Errorf("config %q count = %d, want 1", want, names[want])
		}
	}
	if _, ok := names["notes"]; ok {
		t.Error("notes.txt discovered, want it skipped (wrong extension)")
	}
	if _, ok := names["sub"]; ok {
		t.Error("subdirectory discovered, want it skipped")
	}
	if len(got) != 3 {
		t.Errorf("discovered %d configs, want 3", len(got))
	}
}

func TestWriteCredsFile(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)

	path, err := writeCredsFile("alice", "s3cret")
	if err != nil {
		t.Fatalf("writeCredsFile error: %v", err)
	}
	defer os.Remove(path)

	// The creds file must land on the RAM-backed XDG_RUNTIME_DIR, not on durable
	// storage — that is the whole point of writing it there.
	if dir := filepath.Dir(path); dir != runtime {
		t.Errorf("creds file in %q, want it under XDG_RUNTIME_DIR %q", dir, runtime)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "alice\ns3cret\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// With no XDG_RUNTIME_DIR set, writeCredsFile falls back to the system temp dir
// so a connection can still authenticate.
func TestWriteCredsFileFallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	path, err := writeCredsFile("alice", "s3cret")
	if err != nil {
		t.Fatalf("writeCredsFile error: %v", err)
	}
	defer os.Remove(path)

	if dir := filepath.Dir(path); dir != os.TempDir() {
		t.Errorf("fallback creds file in %q, want system temp %q", dir, os.TempDir())
	}
}
