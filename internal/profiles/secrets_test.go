package profiles

import (
	"errors"
	"testing"
)

type brokenStore struct{}

var errNoService = errors.New("no secret service")

func (brokenStore) Get(string) (string, error) { return "", errNoService }
func (brokenStore) Set(string, string) error   { return errNoService }
func (brokenStore) Delete(string) error        { return errNoService }

func TestFallbackKeepsSecretsForSessionWhenPrimaryFails(t *testing.T) {
	f := WithFallback(brokenStore{})

	if v, err := f.Get("k"); err != nil || v != "" {
		t.Fatalf("Get on empty = %q, %v; want empty, nil", v, err)
	}
	if err := f.Set("k", "secret"); err != nil {
		t.Fatalf("Set = %v, want fallback to memory", err)
	}
	if !f.SessionOnly() {
		t.Fatal("SessionOnly = false after primary failure")
	}
	if v, _ := f.Get("k"); v != "secret" {
		t.Fatalf("Get = %q, want session value", v)
	}
	if err := f.Delete("k"); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if v, _ := f.Get("k"); v != "" {
		t.Fatalf("Get after delete = %q", v)
	}
}

func TestFallbackUsesPrimaryWhenAvailable(t *testing.T) {
	primary := Memory()
	f := WithFallback(primary)

	if err := f.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	if f.SessionOnly() {
		t.Fatal("SessionOnly = true with a working primary")
	}
	if v, _ := primary.Get("k"); v != "v" {
		t.Fatalf("primary has %q, want v", v)
	}
	if v, _ := f.Get("k"); v != "v" {
		t.Fatalf("Get = %q, want v", v)
	}
}
