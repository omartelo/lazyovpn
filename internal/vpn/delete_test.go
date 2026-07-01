package vpn

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// Deleting a connection removes both its file and its saved credentials.
func TestDeleteConfigRemovesFileAndCreds(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "gone")
	if err := SaveCreds("gone", "u", "p"); err != nil {
		t.Fatal(err)
	}

	if err := DeleteConfig(cfg); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}
	if _, err := os.Stat(cfg.Path); !os.IsNotExist(err) {
		t.Error("config file still present after delete")
	}
	if _, _, ok, _ := LoadCreds("gone"); ok {
		t.Error("credentials still present after delete")
	}
}

// Deleting a connection with no saved credentials just removes the file.
func TestDeleteConfigNoCreds(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "plain")

	if err := DeleteConfig(cfg); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}
	if _, err := os.Stat(cfg.Path); !os.IsNotExist(err) {
		t.Error("config file still present after delete")
	}
}

// A file-removal failure aborts before the keyring is touched, so saved
// credentials survive — the connection is still there.
func TestDeleteConfigFileMissingKeepsCreds(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfg := writeNamedConfig(t, dir, "ghost")
	if err := SaveCreds("ghost", "u", "p"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.Path); err != nil { // make DeleteConfig's os.Remove fail
		t.Fatal(err)
	}

	if err := DeleteConfig(cfg); err == nil {
		t.Fatal("DeleteConfig on a missing file: err = nil, want error")
	}
	if _, _, ok, _ := LoadCreds("ghost"); !ok {
		t.Error("credentials dropped despite the file-delete failing")
	}
}
