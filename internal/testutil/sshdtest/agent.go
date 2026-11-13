package sshdtest

import (
	"crypto"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh/agent"
)

// Agent is an in-process ssh-agent served on a unix socket.
type Agent struct {
	Sock    string
	keyring agent.Agent
}

// StartAgent serves an empty keyring on a temp socket until test cleanup.
func StartAgent(t testing.TB) *Agent {
	t.Helper()
	// Unix socket paths are limited to ~104 bytes, so avoid t.TempDir's long names.
	dir, err := os.MkdirTemp("", "ptag")
	if err != nil {
		t.Fatalf("agent tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sock := filepath.Join(dir, "a.sock")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	if err != nil {
		t.Fatalf("agent listen: %v", err)
	}
	a := &Agent{Sock: sock, keyring: agent.NewKeyring()}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		conns []net.Conn
	)
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
			wg.Go(func() { _ = agent.ServeAgent(a.keyring, c) })
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return a
}

// Add loads a private key into the agent.
func (a *Agent) Add(t testing.TB, k crypto.PrivateKey, comment string) {
	t.Helper()
	if err := a.keyring.Add(agent.AddedKey{PrivateKey: k, Comment: comment}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
}

// AddKey loads a generated Key into the agent.
func (a *Agent) AddKey(t testing.TB, k Key) { t.Helper(); a.Add(t, k.Private, "portunus-test") }
