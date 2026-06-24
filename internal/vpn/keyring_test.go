package vpn

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestCredsRoundTrip(t *testing.T) {
	keyring.MockInit()

	if err := SaveCreds("office", "alice", "s3cret:pass"); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	user, pass, ok, err := LoadCreds("office")
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true after save")
	}
	if user != "alice" || pass != "s3cret:pass" {
		t.Errorf("got (%q, %q), want (alice, s3cret:pass)", user, pass)
	}
}

func TestLoadCredsMissing(t *testing.T) {
	keyring.MockInit()

	_, _, ok, err := LoadCreds("never-saved")
	if err != nil {
		t.Errorf("err = %v, want nil for a missing entry", err)
	}
	if ok {
		t.Error("ok = true, want false for a missing entry")
	}
}

// A multiline password must round-trip: Cut splits on the first newline only,
// so everything after it is the password — newlines included.
func TestCredsMultilinePassword(t *testing.T) {
	keyring.MockInit()

	if err := SaveCreds("vpn", "bob", "line1\nline2"); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	user, pass, _, _ := LoadCreds("vpn")
	if user != "bob" || pass != "line1\nline2" {
		t.Errorf("got (%q, %q), want (bob, line1\\nline2)", user, pass)
	}
}

func TestForgetCreds(t *testing.T) {
	keyring.MockInit()

	if err := SaveCreds("temp", "u", "p"); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	if err := ForgetCreds("temp"); err != nil {
		t.Fatalf("ForgetCreds: %v", err)
	}
	if _, _, ok, _ := LoadCreds("temp"); ok {
		t.Error("ok = true after ForgetCreds, want false")
	}
	// Forgetting an absent entry is a no-op, not an error.
	if err := ForgetCreds("temp"); err != nil {
		t.Errorf("ForgetCreds on missing entry: %v, want nil", err)
	}
}
