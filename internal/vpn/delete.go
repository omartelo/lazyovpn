package vpn

import (
	"fmt"
	"os"
)

// DeleteConfig removes a connection: its config file, then its saved
// credentials. The file goes first — it is the operation most likely to fail
// (e.g. a root-owned system-dir config the unprivileged TUI can't touch), and
// on failure nothing else is changed. Once the file is gone the keyring entry
// is dropped best-effort: an orphaned entry is harmless, and a missing one is a
// no-op (see ForgetCreds).
func DeleteConfig(c Config) error {
	if err := os.Remove(c.Path); err != nil {
		return fmt.Errorf("delete config file: %w", err)
	}
	_ = ForgetCreds(c.Name)
	return nil
}
