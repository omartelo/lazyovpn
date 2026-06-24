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

func TestWriteCredsFile(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	path, err := writeCredsFile("alice", "s3cret")
	if err != nil {
		t.Fatalf("writeCredsFile error: %v", err)
	}
	defer os.Remove(path)

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
