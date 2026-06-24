package vpn

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// keyringService namespaces lazyovpn's entries in the OS keyring.
const keyringService = "lazyovpn"

// Credentials are opt-in: the user explicitly chooses to save them. They go to
// the OS keyring (Secret Service / libsecret on Linux) — encrypted at rest and
// unlocked with the login session — never to a plaintext file. The secret blob
// is "username\npassword", matching the credentials-file layout openvpn reads.

// SaveCreds stores a connection's username/password in the OS keyring.
func SaveCreds(name, username, password string) error {
	if err := keyring.Set(keyringService, name, username+"\n"+password); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	return nil
}

// LoadCreds returns saved credentials for a connection. ok is false (with a nil
// error) when nothing is stored — the caller then falls back to prompting.
func LoadCreds(name string) (username, password string, ok bool, err error) {
	secret, err := keyring.Get(keyringService, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load credentials: %w", err)
	}
	// Split on the first newline only, so a password may itself contain newlines.
	username, password, _ = strings.Cut(secret, "\n")
	return username, password, true, nil
}

// ForgetCreds removes a connection's saved credentials. A missing entry is not
// an error — forgetting what was never saved is a no-op.
func ForgetCreds(name string) error {
	err := keyring.Delete(keyringService, name)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("forget credentials: %w", err)
	}
	return nil
}
