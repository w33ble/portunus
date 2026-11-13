// Package settings persists user preferences as JSON in the app config dir.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const fileName = "settings.json"

// Settings are user-tunable preferences. Zero values are replaced by Defaults on load.
type Settings struct {
	// GlobalTransfers caps concurrently running transfer tasks.
	GlobalTransfers int `json:"globalTransfers"`
	// PerConnectionTransfers caps tasks touching any single connection.
	PerConnectionTransfers int `json:"perConnectionTransfers"`
	// LoadAppleKeychain runs `ssh-add --apple-load-keychain` before passphrase prompts (macOS).
	LoadAppleKeychain bool `json:"loadAppleKeychain"`
	// ShowHidden lists dotfiles in panes.
	ShowHidden bool `json:"showHidden"`
}

// Defaults are applied for missing or invalid fields.
func Defaults() Settings {
	return Settings{
		GlobalTransfers:        4,
		PerConnectionTransfers: 2,
		LoadAppleKeychain:      true,
		ShowHidden:             false,
	}
}

// Store reads and writes settings.json. Safe for concurrent use.
type Store struct {
	path string

	mu  sync.Mutex
	cur Settings
}

// Open loads dir/settings.json, using defaults when the file does not exist.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, fileName), cur: Defaults()}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}

	// Unmarshal over defaults so fields added later keep their default value.
	loaded := Defaults()
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	s.cur = normalize(loaded)
	return s, nil
}

func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Save normalizes, persists atomically, and returns what was stored.
func (s *Store) Save(next Settings) (Settings, error) {
	next = normalize(next)

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return Settings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := writeAtomic(s.path, data); err != nil {
		return Settings{}, err
	}
	s.cur = next
	return next, nil
}

func normalize(v Settings) Settings {
	d := Defaults()
	if v.GlobalTransfers < 1 {
		v.GlobalTransfers = d.GlobalTransfers
	}
	if v.PerConnectionTransfers < 1 {
		v.PerConnectionTransfers = d.PerConnectionTransfers
	}
	v.GlobalTransfers = min(v.GlobalTransfers, 32)
	v.PerConnectionTransfers = min(v.PerConnectionTransfers, v.GlobalTransfers)
	return v
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}
