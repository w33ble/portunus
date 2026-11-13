// Package state persists UI state that is restored on launch (today the main window's frame)
// as JSON in the app state dir. Unlike settings it is disposable: an unreadable file is dropped.
package state

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/w33ble/portunus/internal/atomicfile"
)

const fileName = "state.json"

// Point is a position in screen coordinates (DIP, origin at the primary screen's top-left).
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Window is the main window's "normal" frame: where it sits when neither maximised nor fullscreen.
type Window struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// Position is nil when the platform cannot report or set window positions (Wayland).
	Position *Point `json:"position,omitzero"`
	// Maximised is whether the window was maximised when last seen.
	Maximised bool `json:"maximised,omitzero"`
}

func (w Window) equal(o Window) bool {
	if (w.Position == nil) != (o.Position == nil) || (w.Position != nil && *w.Position != *o.Position) {
		return false
	}
	return w.Width == o.Width && w.Height == o.Height && w.Maximised == o.Maximised
}

type fileData struct {
	Window *Window `json:"window,omitzero"`
}

// Store reads and writes state.json. Safe for concurrent use.
type Store struct {
	path string

	mu   sync.Mutex
	data fileData
}

// Open loads dir/state.json. A missing or unparsable file is an empty store; the next write replaces it.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, fileName)}

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		s.data = fileData{}
	}
	return s, nil
}

// Window returns the saved window frame, if any.
func (s *Store) Window() (Window, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Window == nil {
		return Window{}, false
	}
	return *s.data.Window, true
}

// SetWindow persists w atomically.
func (s *Store) SetWindow(w Window) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := fileData{Window: &w}
	raw, err := json.Marshal(next, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	if err := atomicfile.Write(s.path, append(raw, '\n')); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	s.data = next
	return nil
}
