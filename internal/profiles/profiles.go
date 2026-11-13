// Package profiles stores saved S3 connection profiles. Secrets live in a SecretStore, never in JSON.
package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/w33ble/portunus/internal/atomicfile"
	"github.com/w33ble/portunus/internal/vfs/s3fs"
)

const fileName = "profiles.json"

// S3Profile is a saved S3 connection. The secret access key is held in the SecretStore.
type S3Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Profile     string `json:"profile,omitzero"`
	AccessKeyID string `json:"accessKeyId,omitzero"`
	Region      string `json:"region,omitzero"`
	Endpoint    string `json:"endpoint,omitzero"`
	PathStyle   bool   `json:"pathStyle,omitzero"`
	Bucket      string `json:"bucket,omitzero"`
}

type fileData struct {
	S3 []S3Profile `json:"s3"`
}

// Store persists profiles in <dir>/profiles.json.
type Store struct {
	path    string
	secrets SecretStore

	mu       sync.Mutex
	profiles []S3Profile
}

// Open loads the store, creating dir if needed. A missing file is an empty store.
func Open(dir string, secrets SecretStore) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("profiles: create %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, fileName), secrets: secrets}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("profiles: read %s: %w", s.path, err)
	default:
		var d fileData
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("profiles: parse %s: %w", s.path, err)
		}
		s.profiles = d.S3
	}
	return s, nil
}

func secretKey(id string) string { return "s3:" + id }

func (s *Store) List() []S3Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.profiles)
}

func (s *Store) Get(id string) (S3Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(id)
}

func (s *Store) get(id string) (S3Profile, bool) {
	for _, p := range s.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return S3Profile{}, false
}

// Save creates or updates a profile, assigning an ID when empty. An empty secret keeps the stored one.
func (s *Store) Save(p S3Profile, secret string) (S3Profile, error) {
	if p.Name == "" {
		return S3Profile{}, errors.New("profiles: name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		id, err := newID()
		if err != nil {
			return S3Profile{}, err
		}
		p.ID = id
	}
	if secret != "" {
		if err := s.secrets.Set(secretKey(p.ID), secret); err != nil {
			return S3Profile{}, fmt.Errorf("profiles: store secret: %w", err)
		}
	}
	next := slices.Clone(s.profiles)
	if i := slices.IndexFunc(next, func(q S3Profile) bool { return q.ID == p.ID }); i >= 0 {
		next[i] = p
	} else {
		next = append(next, p)
	}
	if err := s.write(next); err != nil {
		return S3Profile{}, err
	}
	s.profiles = next
	return p, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.profiles, func(q S3Profile) bool { return q.ID == id })
	if i < 0 {
		return fmt.Errorf("profiles: %s: %w", id, os.ErrNotExist)
	}
	next := slices.Delete(slices.Clone(s.profiles), i, i+1)
	if err := s.write(next); err != nil {
		return err
	}
	s.profiles = next
	if err := s.secrets.Delete(secretKey(id)); err != nil {
		return fmt.Errorf("profiles: delete secret: %w", err)
	}
	return nil
}

// S3Config builds an s3fs.Config for the profile, filling the secret from the SecretStore.
func (s *Store) S3Config(id string) (s3fs.Config, error) {
	p, ok := s.Get(id)
	if !ok {
		return s3fs.Config{}, fmt.Errorf("profiles: %s: %w", id, os.ErrNotExist)
	}
	secret, err := s.secrets.Get(secretKey(id))
	if err != nil {
		return s3fs.Config{}, fmt.Errorf("profiles: load secret: %w", err)
	}
	if p.AccessKeyID != "" && p.Profile == "" && secret == "" {
		return s3fs.Config{}, fmt.Errorf("profiles: %s: secret access key is missing from the keychain; re-enter it", id)
	}
	return s3fs.Config{
		Profile:         p.Profile,
		AccessKeyID:     p.AccessKeyID,
		SecretAccessKey: secret,
		Region:          p.Region,
		Endpoint:        p.Endpoint,
		PathStyle:       p.PathStyle,
		Bucket:          p.Bucket,
	}, nil
}

func (s *Store) write(list []S3Profile) error {
	if list == nil {
		list = []S3Profile{}
	}
	b, err := json.Marshal(fileData{S3: list}, jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("profiles: encode: %w", err)
	}
	if err := atomicfile.Write(s.path, b); err != nil {
		return fmt.Errorf("profiles: write %s: %w", s.path, err)
	}
	return nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("profiles: id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
