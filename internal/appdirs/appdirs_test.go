package appdirs

import (
	"path/filepath"
	"testing"
)

func TestDirs(t *testing.T) {
	home := t.TempDir()
	xdgConfig := filepath.Join(t.TempDir(), "cfg")
	xdgState := filepath.Join(t.TempDir(), "st")

	tests := []struct {
		name       string
		config     string
		state      string
		wantConfig string
		wantState  string
	}{
		{
			name:       "XDG variables win",
			config:     xdgConfig,
			state:      xdgState,
			wantConfig: filepath.Join(xdgConfig, "portunus"),
			wantState:  filepath.Join(xdgState, "portunus"),
		},
		{
			name:       "unset falls back to home",
			wantConfig: filepath.Join(home, ".config", "portunus"),
			wantState:  filepath.Join(home, ".local", "state", "portunus"),
		},
		{
			name:       "relative values are ignored",
			config:     "relative/config",
			state:      "./state",
			wantConfig: filepath.Join(home, ".config", "portunus"),
			wantState:  filepath.Join(home, ".local", "state", "portunus"),
		},
		{
			name:       "each variable is independent",
			config:     xdgConfig,
			wantConfig: filepath.Join(xdgConfig, "portunus"),
			wantState:  filepath.Join(home, ".local", "state", "portunus"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", tc.config)
			t.Setenv("XDG_STATE_HOME", tc.state)

			if got, err := ConfigDir(); err != nil || got != tc.wantConfig {
				t.Errorf("ConfigDir() = %q, %v; want %q", got, err, tc.wantConfig)
			}
			if got, err := StateDir(); err != nil || got != tc.wantState {
				t.Errorf("StateDir() = %q, %v; want %q", got, err, tc.wantState)
			}
		})
	}
}

func TestNoHomeIsAnError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	if got, err := ConfigDir(); err == nil {
		t.Errorf("ConfigDir() = %q, want error without a home directory", got)
	}
	if got, err := StateDir(); err == nil {
		t.Errorf("StateDir() = %q, want error without a home directory", got)
	}
}
