// Package applog configures lazyovpn's application log: a per-run logfmt file
// under the user's XDG state dir, readable with `lazyovpn log`. slog's
// TextHandler already emits logfmt, so there's no logging dependency.
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// keepLogs is how many per-run log files to retain; older ones are pruned on
// startup. A fresh file per run grows the directory without bound otherwise.
const keepLogs = 10

// filePrefix / fileSuffix bracket a per-run log filename. The middle is a
// timestamp (see Setup) so names sort chronologically.
const (
	filePrefix = "lazyovpn-"
	fileSuffix = ".log"
)

// LogDir is where per-run log files live: $XDG_STATE_HOME/lazyovpn/logs, or
// ~/.local/state/lazyovpn/logs when XDG_STATE_HOME is unset. Logs are state,
// not config, so they go under the state dir (not ~/.config).
func LogDir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "lazyovpn", "logs"), nil
}

// Setup creates the log directory, prunes old runs, opens a fresh per-run file,
// and installs a file-only slog default (logfmt via TextHandler). It returns
// the file for the caller to close on exit. File-only matters: the TUI owns the
// terminal, so a stray stderr write would corrupt the alt screen.
func Setup() (io.Closer, error) {
	dir, err := LogDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	prune(dir, keepLogs)

	name := filePrefix + time.Now().Format("20060102-150405") + fileSuffix
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, privateFileMode)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	// AddSource stamps each line with file:line — this is a debug log, so knowing
	// where an event came from is the whole point.
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})))
	return f, nil
}

// privateFileMode keeps logs owner-only: openvpn output can carry the server
// address and other network detail; no reason to make it world-readable.
const privateFileMode os.FileMode = 0o600

// Latest returns the path of the newest per-run log file, or an error if none
// exist. The timestamp naming sorts lexically, so the last name is the newest.
func Latest() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	files, err := logFiles(dir)
	if err != nil {
		return "", fmt.Errorf("read log dir: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no log files in %s", dir)
	}
	return filepath.Join(dir, files[len(files)-1]), nil
}

// logFiles returns lazyovpn's per-run log filenames in dir, sorted ascending
// (oldest first). Only files matching the run-log naming are considered, so a
// stray file in the dir is never touched or reported.
func logFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, filePrefix) && strings.HasSuffix(n, fileSuffix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

// prune deletes all but the newest keep run-log files in dir. Best-effort: a
// missing dir or an un-removable file is ignored — pruning must never block
// startup.
func prune(dir string, keep int) {
	files, err := logFiles(dir)
	if err != nil {
		return
	}
	for i := 0; i < len(files)-keep; i++ {
		_ = os.Remove(filepath.Join(dir, files[i]))
	}
}
