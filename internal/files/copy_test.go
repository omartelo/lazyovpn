package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopy(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.ovpn")
	if err := os.WriteFile(src, []byte("client\nremote x 1194\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst.ovpn")

	if err := Copy(src, dst, 0o600); err != nil {
		t.Fatalf("Copy error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "client\nremote x 1194\n" {
		t.Errorf("content = %q", got)
	}
	if info, _ := os.Stat(dst); info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestCopyForcesPermOnOverwrite(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.ovpn")
	os.WriteFile(src, []byte("a"), 0o644)
	dst := filepath.Join(t.TempDir(), "dst.ovpn")
	os.WriteFile(dst, []byte("old"), 0o644) // pre-exists at 0644

	if err := Copy(src, dst, 0o600); err != nil {
		t.Fatalf("Copy error: %v", err)
	}
	if info, _ := os.Stat(dst); info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o after overwrite, want 600", info.Mode().Perm())
	}
}

func TestCopyMissingSource(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst.ovpn")
	if err := Copy("/nonexistent/src.ovpn", dst, 0o600); err == nil {
		t.Error("Copy with missing source: want error, got nil")
	}
}
