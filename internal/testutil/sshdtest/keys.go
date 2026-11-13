package sshdtest

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Key is a generated client or host key pair.
type Key struct {
	Private crypto.PrivateKey
	Public  ssh.PublicKey
	Signer  ssh.Signer
}

// NewKey generates an ed25519 key.
func NewKey(t testing.TB) Key {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return newKey(t, priv)
}

// NewRSAKey generates an RSA key (exercises rsa-sha2 signing paths).
func NewRSAKey(t testing.TB, bits int) Key {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return newKey(t, priv)
}

func newKey(t testing.TB, priv crypto.PrivateKey) Key {
	t.Helper()
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer from key: %v", err)
	}
	return Key{Private: priv, Public: signer.PublicKey(), Signer: signer}
}

// AuthorizedLine returns the key as an authorized_keys line.
func (k Key) AuthorizedLine() string { return string(ssh.MarshalAuthorizedKey(k.Public)) }

// WriteFiles writes the OpenSSH-format private key to dir/name (0600), encrypted when
// passphrase is non-empty, and dir/name.pub when pub is true. It returns the private path.
func (k Key) WriteFiles(t testing.TB, dir, name, passphrase string, pub bool) string {
	t.Helper()
	var (
		block *pem.Block
		err   error
	)
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(k.Private, "portunus-test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(k.Private, "portunus-test", []byte(passphrase))
	}
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if pub {
		if err := os.WriteFile(p+".pub", ssh.MarshalAuthorizedKey(k.Public), 0o600); err != nil {
			t.Fatalf("write pub key: %v", err)
		}
	}
	return p
}

// WritePub writes only dir/name.pub (the 1Password/Secretive "agent key selector" layout)
// and returns its path.
func (k Key) WritePub(t testing.TB, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name+".pub")
	if err := os.WriteFile(p, ssh.MarshalAuthorizedKey(k.Public), 0o600); err != nil {
		t.Fatalf("write pub key: %v", err)
	}
	return p
}
