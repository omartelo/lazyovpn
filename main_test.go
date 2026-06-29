package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// lookFrom returns a stub LookPath that reports the given names as installed
// (resolving to /usr/bin/<name>) and everything else as missing.
func lookFrom(installed ...string) func(string) (string, error) {
	set := make(map[string]bool, len(installed))
	for _, n := range installed {
		set[n] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
}

func TestRunDoctor(t *testing.T) {
	tests := []struct {
		name      string
		installed []string
		wantOK    bool
		// substrings that must / must not appear in the report
		want    []string
		notWant []string
	}{
		{
			name:      "all present",
			installed: []string{"openvpn", "pkexec", "zenity"},
			wantOK:    true,
			want:      []string{"✓ openvpn — /usr/bin/openvpn", "✓ pkexec", "✓ file chooser — /usr/bin/zenity"},
			notWant:   []string{"✗", "⚠"},
		},
		{
			name:      "openvpn missing fails",
			installed: []string{"pkexec", "zenity"},
			wantOK:    false,
			want:      []string{"✗ openvpn — missing (required)"},
		},
		{
			name:      "pkexec missing fails",
			installed: []string{"openvpn", "zenity"},
			wantOK:    false,
			want:      []string{"✗ pkexec — missing (required)"},
		},
		{
			name:      "chooser missing is only a warning",
			installed: []string{"openvpn", "pkexec"},
			wantOK:    true,
			want:      []string{"⚠ file chooser — missing (optional)"},
			notWant:   []string{"✗"},
		},
		{
			name:      "second chooser backend counts",
			installed: []string{"openvpn", "pkexec", "kdialog"},
			wantOK:    true,
			want:      []string{"✓ file chooser — /usr/bin/kdialog"},
			notWant:   []string{"⚠", "✗"},
		},
		{
			name:      "nothing installed fails",
			installed: nil,
			wantOK:    false,
			want:      []string{"✗ openvpn", "✗ pkexec", "⚠ file chooser"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, ok := runDoctor(lookFrom(tt.installed...))
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v\nreport:\n%s", ok, tt.wantOK, report)
			}
			for _, s := range tt.want {
				if !strings.Contains(report, s) {
					t.Errorf("report missing %q\ngot:\n%s", s, report)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(report, s) {
					t.Errorf("report should not contain %q\ngot:\n%s", s, report)
				}
			}
		})
	}
}

// runDoctor must report a non-nil LookPath error as "missing", not crash.
func TestRunDoctorTreatsErrAsMissing(t *testing.T) {
	look := func(string) (string, error) { return "", errors.New("boom") }
	report, ok := runDoctor(look)
	if ok {
		t.Fatalf("ok = true, want false when nothing resolves\nreport:\n%s", report)
	}
}
