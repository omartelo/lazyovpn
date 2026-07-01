package vpn

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenameConfig renames a connection. A connection's name IS its config filename,
// so this renames the file on disk (keeping its extension) and returns the
// updated Config.
//
// Credentials are migrated first: if the old name has an entry in the keyring it
// is copied to the new name BEFORE the file moves, so a keyring failure aborts
// the rename with nothing changed (the caller reports it and stops). Only once
// the migration and the file rename both succeed is the old keyring entry
// removed; a file-rename failure rolls the migrated entry back. A config with no
// saved credentials just gets its file renamed.
//
// It refuses a name that is not a bare filename (empty, "."/"..", or containing
// a path separator — traversal) and refuses to overwrite an existing config.
func RenameConfig(c Config, newName string) (Config, error) {
	newName = strings.TrimSpace(newName)
	if err := validConnName(newName); err != nil {
		return Config{}, err
	}
	if newName == c.Name {
		return c, nil // no change requested
	}

	newPath := filepath.Join(filepath.Dir(c.Path), newName+filepath.Ext(c.Path))
	if _, err := os.Stat(newPath); err == nil {
		return Config{}, fmt.Errorf("a connection named %q already exists", newName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("check target path: %w", err)
	}

	// Migrate saved credentials before touching the file. A keyring read or write
	// failure here means we stop with the file and the old entry untouched.
	user, pass, hasCreds, err := LoadCreds(c.Name)
	if err != nil {
		return Config{}, fmt.Errorf("migrate credentials: %w", err)
	}
	if hasCreds {
		if err := SaveCreds(newName, user, pass); err != nil {
			return Config{}, fmt.Errorf("migrate credentials: %w", err)
		}
	}

	if err := os.Rename(c.Path, newPath); err != nil {
		if hasCreds {
			_ = ForgetCreds(newName) // roll the migration back
		}
		return Config{}, fmt.Errorf("rename config file: %w", err)
	}

	if hasCreds {
		_ = ForgetCreds(c.Name) // best-effort: the new entry is authoritative now
	}
	return Config{Name: newName, Path: newPath}, nil
}

// validConnName rejects a connection name that would not be a safe bare
// filename: empty, "."/"..", or containing a path separator (traversal).
func validConnName(name string) error {
	if name == "" {
		return errors.New("connection name cannot be empty")
	}
	if name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, '/') {
		return fmt.Errorf("invalid connection name %q", name)
	}
	return nil
}
