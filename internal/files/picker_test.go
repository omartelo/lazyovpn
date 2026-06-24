package files

import (
	"os"
	"path/filepath"
	"testing"
)

func writeExec(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveChooser(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // only this dir is searched

	if _, ok := resolveChooser(); ok {
		t.Fatal("resolveChooser: want none with an empty PATH")
	}

	writeExec(t, filepath.Join(dir, "kdialog"))
	if c, ok := resolveChooser(); !ok || c.bin != "kdialog" {
		t.Fatalf("resolveChooser = %q, %v; want kdialog, true", c.bin, ok)
	}

	// zenity present too → preferred (first entry in the list wins).
	writeExec(t, filepath.Join(dir, "zenity"))
	if c, _ := resolveChooser(); c.bin != "zenity" {
		t.Fatalf("resolveChooser = %q; want zenity (first available)", c.bin)
	}
}
