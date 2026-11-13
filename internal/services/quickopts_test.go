package services

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestQuickOptionsDefaults(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"id_rsa", "id_ed25519"} {
		if err := os.WriteFile(filepath.Join(sshDir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	o := quickOptions(Target{Kind: TargetSSH, Host: "example.com"}, home, "me")
	if o.HostName != "example.com" || o.User != "me" || o.Port != 22 || o.StrictHostKeyChecking != "ask" {
		t.Fatalf("options = %+v", o)
	}
	wantIDs := []string{filepath.Join(sshDir, "id_ed25519"), filepath.Join(sshDir, "id_rsa")}
	if !reflect.DeepEqual(o.IdentityFiles, wantIDs) {
		t.Errorf("identities = %v, want %v", o.IdentityFiles, wantIDs)
	}
	if !reflect.DeepEqual(o.UserKnownHostsFiles, []string{filepath.Join(sshDir, "known_hosts")}) {
		t.Errorf("known hosts = %v", o.UserKnownHostsFiles)
	}
}

func TestQuickOptionsExplicitUserAndPort(t *testing.T) {
	o := quickOptions(Target{Kind: TargetSSH, Host: "h", User: "bob", Port: 2222}, t.TempDir(), "me")
	if o.User != "bob" || o.Port != 2222 || len(o.IdentityFiles) != 0 {
		t.Fatalf("options = %+v", o)
	}
}
