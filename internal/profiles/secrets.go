package profiles

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// SecretStore holds secrets by key. Get on a missing key returns ("", nil).
type SecretStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

type keyringStore struct{ service string }

// Keyring returns a SecretStore backed by the OS keychain.
func Keyring(service string) SecretStore { return keyringStore{service} }

func (k keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return v, err
}

func (k keyringStore) Set(key, value string) error { return keyring.Set(k.service, key, value) }

func (k keyringStore) Delete(key string) error {
	err := keyring.Delete(k.service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

type memoryStore struct {
	mu sync.Mutex
	m  map[string]string
}

// Memory returns a session-only SecretStore.
func Memory() SecretStore { return &memoryStore{m: map[string]string{}} }

func (s *memoryStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key], nil
}

func (s *memoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

func (s *memoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// FallbackStore writes to primary and falls back to session memory when primary is
// unavailable (e.g. Linux without a Secret Service), so the app still works for the session.
type FallbackStore struct {
	primary SecretStore
	mem     SecretStore

	mu       sync.Mutex
	degraded bool
}

func WithFallback(primary SecretStore) *FallbackStore {
	return &FallbackStore{primary: primary, mem: Memory()}
}

// SessionOnly reports whether a secret had to be kept in memory because primary failed.
func (f *FallbackStore) SessionOnly() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.degraded
}

func (f *FallbackStore) Get(key string) (string, error) {
	if v, _ := f.mem.Get(key); v != "" {
		return v, nil
	}
	return f.primary.Get(key)
}

func (f *FallbackStore) Set(key, value string) error {
	if err := f.primary.Set(key, value); err != nil {
		f.mu.Lock()
		f.degraded = true
		f.mu.Unlock()
		return f.mem.Set(key, value)
	}
	return f.mem.Delete(key)
}

func (f *FallbackStore) Delete(key string) error {
	_ = f.mem.Delete(key)
	if err := f.primary.Delete(key); err != nil && !f.SessionOnly() {
		return err
	}
	return nil
}
