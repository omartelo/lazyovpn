package vpn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

// writeNamedConfig drops a named config file in dir and returns its Config.
func writeNamedConfig(t *testing.T, dir, name string) Config {
	t.Helper()
	path := filepath.Join(dir, name+".ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Config{Name: name, Path: path}
}

// A rename with no saved credentials just moves the file and keeps its extension.
func TestRenameConfigNoCreds(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "old")

	got, err := RenameConfig(cfg, "new")
	if err != nil {
		t.Fatalf("RenameConfig: %v", err)
	}
	if got.Name != "new" {
		t.Errorf("Name = %q, want new", got.Name)
	}
	if want := filepath.Join(dir, "new.ovpn"); got.Path != want {
		t.Errorf("Path = %q, want %q", got.Path, want)
	}
	if _, err := os.Stat(cfg.Path); !os.IsNotExist(err) {
		t.Error("old file still present after rename")
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Errorf("new file missing: %v", err)
	}
}

// A rename migrates saved credentials to the new name and drops the old entry.
func TestRenameConfigMigratesCreds(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "old")
	if err := SaveCreds("old", "alice", "s3cret"); err != nil {
		t.Fatal(err)
	}

	got, err := RenameConfig(cfg, "new")
	if err != nil {
		t.Fatalf("RenameConfig: %v", err)
	}

	user, pass, ok, _ := LoadCreds("new")
	if !ok || user != "alice" || pass != "s3cret" {
		t.Errorf("new creds = (%q, %q, %v), want (alice, s3cret, true)", user, pass, ok)
	}
	if _, _, ok, _ := LoadCreds("old"); ok {
		t.Error("old creds still present after migration")
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Errorf("new file missing: %v", err)
	}
}

// A rename onto an existing config name is refused, leaving both files and any
// saved credentials untouched.
func TestRenameConfigTargetExists(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "old")
	writeNamedConfig(t, dir, "taken")
	if err := SaveCreds("old", "u", "p"); err != nil {
		t.Fatal(err)
	}

	if _, err := RenameConfig(cfg, "taken"); err == nil {
		t.Fatal("RenameConfig onto an existing name: err = nil, want error")
	}
	if _, err := os.Stat(cfg.Path); err != nil {
		t.Error("source file gone after a refused rename")
	}
	if _, _, ok, _ := LoadCreds("old"); !ok {
		t.Error("creds migrated despite a refused rename")
	}
	if _, _, ok, _ := LoadCreds("taken"); ok {
		t.Error("creds written under the target name on a refused rename")
	}
}

// Invalid names (empty, traversal, dot) are refused before anything is touched.
func TestRenameConfigInvalidNames(t *testing.T) {
	keyring.MockInit()
	for _, name := range []string{"", "   ", ".", "..", "a/b", "../evil"} {
		dir := t.TempDir()
		cfg := writeNamedConfig(t, dir, "old")
		if _, err := RenameConfig(cfg, name); err == nil {
			t.Errorf("RenameConfig(%q): err = nil, want error", name)
		}
		if _, err := os.Stat(cfg.Path); err != nil {
			t.Errorf("source file gone after refusing name %q", name)
		}
	}
}

// Renaming to the current name is a no-op that succeeds and changes nothing.
func TestRenameConfigSameName(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "same")

	got, err := RenameConfig(cfg, "same")
	if err != nil {
		t.Fatalf("RenameConfig same name: %v", err)
	}
	if got != cfg {
		t.Errorf("got %+v, want unchanged %+v", got, cfg)
	}
}

// A file-rename failure after the credentials were migrated rolls the migration
// back: the new entry is removed and the old one stays, so nothing is lost.
func TestRenameConfigRollsBackOnRenameFailure(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "old")
	if err := SaveCreds("old", "u", "p"); err != nil {
		t.Fatal(err)
	}
	// Remove the source file so os.Rename fails after the keyring migration.
	if err := os.Remove(cfg.Path); err != nil {
		t.Fatal(err)
	}

	if _, err := RenameConfig(cfg, "new"); err == nil {
		t.Fatal("RenameConfig with a missing source: err = nil, want error")
	}
	if _, _, ok, _ := LoadCreds("new"); ok {
		t.Error("migrated creds not rolled back after the rename failed")
	}
	if _, _, ok, _ := LoadCreds("old"); !ok {
		t.Error("old creds lost after a failed rename")
	}
}
