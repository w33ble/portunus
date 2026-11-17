package connprefs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/w33ble/portunus/internal/atomicfile"
)

const fileName = "connections.json"

// fileData keeps top-level sections written by a newer version, so a rewrite doesn't drop them.
type fileData struct {
	Data
	Unknown jsontext.Value `json:",embed"`
}

// Store persists Data in <dir>/connections.json. Safe for concurrent use.
type Store struct {
	path string

	mu      sync.Mutex
	data    Data
	unknown jsontext.Value
}

// Open loads the store, creating dir if needed. A missing file is an empty store.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("connprefs: create %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, fileName)}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("connprefs: read %s: %w", s.path, err)
	default:
		var f fileData
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("connprefs: parse %s: %w", s.path, err)
		}
		s.data, s.unknown = f.Data, f.Unknown
		for i, h := range s.data.Hosts {
			s.data.Hosts[i].ConnID = SSHConnID(h.User, h.Host, h.Port)
		}
	}
	s.data = s.data.clone()
	return s, nil
}

// Data returns a copy of everything stored.
func (s *Store) Data() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.clone()
}

// SaveHost creates or updates a saved host, assigning an ID when empty. When an edit
// changes the host's connection ID, its preferences and bookmarks move with it.
func (s *Store) SaveHost(h Host) (Host, error) {
	h.Name = strings.TrimSpace(h.Name)
	h.User = strings.TrimSpace(h.User)
	h.Host = strings.TrimSpace(h.Host)
	h.ConnID = SSHConnID(h.User, h.Host, h.Port)
	switch {
	case h.Name == "":
		return Host{}, errors.New("connprefs: host name is required")
	case h.Host == "":
		return Host{}, errors.New("connprefs: host address is required")
	case h.Port < 0 || h.Port > 65535:
		return Host{}, fmt.Errorf("connprefs: port %d is out of range", h.Port)
	}

	err := s.update(func(d *Data) error {
		connID := h.ConnID
		if i := slices.IndexFunc(d.Hosts, func(o Host) bool { return o.ID != h.ID && o.ConnID == connID }); i >= 0 {
			return fmt.Errorf("connprefs: %s is already saved as %q", strings.TrimPrefix(connID, "ssh:"), d.Hosts[i].Name)
		}

		if h.ID == "" {
			id, err := newID()
			if err != nil {
				return err
			}
			h.ID = id
			d.Hosts = append(d.Hosts, h)
			return nil
		}

		i := slices.IndexFunc(d.Hosts, func(o Host) bool { return o.ID == h.ID })
		if i < 0 {
			return fmt.Errorf("connprefs: host %s: %w", h.ID, ErrNotFound)
		}
		d.rekey(d.Hosts[i].ConnID, connID)
		d.Hosts[i] = h
		return nil
	})
	if err != nil {
		return Host{}, err
	}
	return h, nil
}

// DeleteHost removes a saved host with its preferences and bookmarks.
func (s *Store) DeleteHost(id string) error {
	return s.update(func(d *Data) error {
		i := slices.IndexFunc(d.Hosts, func(o Host) bool { return o.ID == id })
		if i < 0 {
			return fmt.Errorf("connprefs: host %s: %w", id, ErrNotFound)
		}
		d.forget(d.Hosts[i].ConnID)
		d.Hosts = slices.Delete(d.Hosts, i, i+1)
		return nil
	})
}

// SetConnection replaces one connection's preferences and bookmarks. New bookmarks get IDs,
// and a second bookmark for the same folder is dropped.
func (s *Store) SetConnection(connID string, p Prefs, bookmarks []Bookmark) error {
	if connID == "" {
		return errors.New("connprefs: connection ID is required")
	}
	p, err := normalizePrefs(p)
	if err != nil {
		return err
	}

	keep := make([]Bookmark, 0, len(bookmarks))
	for _, b := range bookmarks {
		b.ConnID = connID
		if b, err = normalizeBookmark(b); err != nil {
			return err
		}
		if !slices.ContainsFunc(keep, func(o Bookmark) bool { return o.Path == b.Path }) {
			keep = append(keep, b)
		}
	}

	return s.update(func(d *Data) error {
		d.Bookmarks = slices.DeleteFunc(d.Bookmarks, func(b Bookmark) bool { return b.ConnID == connID })
		for i := range keep {
			if keep[i].ID == "" {
				id, err := newID()
				if err != nil {
					return err
				}
				keep[i].ID = id
			}
		}
		d.Bookmarks = append(d.Bookmarks, keep...)
		if p == (Prefs{}) {
			delete(d.Prefs, connID)
		} else {
			d.Prefs[connID] = p
		}
		return nil
	})
}

// AddBookmark saves a folder. Bookmarking a folder twice returns the existing bookmark.
func (s *Store) AddBookmark(b Bookmark) (Bookmark, error) {
	if b.ConnID == "" {
		return Bookmark{}, errors.New("connprefs: connection ID is required")
	}
	b, err := normalizeBookmark(b)
	if err != nil {
		return Bookmark{}, err
	}

	err = s.update(func(d *Data) error {
		if i := slices.IndexFunc(d.Bookmarks, func(o Bookmark) bool { return o.ConnID == b.ConnID && o.Path == b.Path }); i >= 0 {
			b = d.Bookmarks[i]
			return nil
		}
		id, err := newID()
		if err != nil {
			return err
		}
		b.ID = id
		d.Bookmarks = append(d.Bookmarks, b)
		return nil
	})
	if err != nil {
		return Bookmark{}, err
	}
	return b, nil
}

func (s *Store) RemoveBookmark(id string) error {
	return s.update(func(d *Data) error {
		i := slices.IndexFunc(d.Bookmarks, func(o Bookmark) bool { return o.ID == id })
		if i < 0 {
			return fmt.Errorf("connprefs: bookmark %s: %w", id, ErrNotFound)
		}
		d.Bookmarks = slices.Delete(d.Bookmarks, i, i+1)
		return nil
	})
}

// Forget drops a connection's preferences and bookmarks, e.g. when its S3 profile is deleted.
func (s *Store) Forget(connID string) error {
	return s.update(func(d *Data) error {
		d.forget(connID)
		return nil
	})
}

// Prune drops preferences and bookmarks of connections keep rejects, such as an alias
// removed from ~/.ssh/config. It writes only when something was dropped.
func (s *Store) Prune(keep func(connID string) bool) error {
	s.mu.Lock()
	stale := slices.ContainsFunc(s.data.Bookmarks, func(b Bookmark) bool { return !keep(b.ConnID) })
	for id := range s.data.Prefs {
		stale = stale || !keep(id)
	}
	s.mu.Unlock()
	if !stale {
		return nil
	}

	return s.update(func(d *Data) error {
		maps.DeleteFunc(d.Prefs, func(id string, _ Prefs) bool { return !keep(id) })
		d.Bookmarks = slices.DeleteFunc(d.Bookmarks, func(b Bookmark) bool { return !keep(b.ConnID) })
		return nil
	})
}

// update applies fn to a copy of the data and keeps it only once it's written.
func (s *Store) update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data.clone()
	if err := fn(&next); err != nil {
		return err
	}
	b, err := json.Marshal(fileData{Data: next, Unknown: s.unknown}, jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("connprefs: encode: %w", err)
	}
	if err := atomicfile.Write(s.path, append(b, '\n')); err != nil {
		return fmt.Errorf("connprefs: write %s: %w", s.path, err)
	}
	s.data = next
	return nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("connprefs: id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
