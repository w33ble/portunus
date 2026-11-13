// Package appdirs resolves where Portunus keeps its files. The XDG layout is used on every
// platform, macOS included, instead of os.UserConfigDir (~/Library/Application Support).
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "portunus"

// ConfigDir holds settings.json, profiles.json and the fallback known_hosts:
// $XDG_CONFIG_HOME/portunus, else ~/.config/portunus.
func ConfigDir() (string, error) {
	return resolve("XDG_CONFIG_HOME", filepath.Join(".config", appName))
}

// StateDir holds state.json (window geometry): $XDG_STATE_HOME/portunus, else ~/.local/state/portunus.
func StateDir() (string, error) {
	return resolve("XDG_STATE_HOME", filepath.Join(".local", "state", appName))
}

// resolve honors an XDG variable only when it is an absolute path; the spec says to ignore relative values.
func resolve(envVar, homeRelative string) (string, error) {
	if base := os.Getenv(envVar); filepath.IsAbs(base) {
		return filepath.Join(base, appName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("appdirs: locate home directory: %w", err)
	}
	return filepath.Join(home, homeRelative), nil
}
