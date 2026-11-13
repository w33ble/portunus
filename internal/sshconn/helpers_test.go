package sshconn_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/testutil/sshdtest"
)

// fakePrompter records every question and answers through the configured funcs.
type fakePrompter struct {
	mu       sync.Mutex
	secrets  []prompt.SecretRequest
	hostKeys []prompt.HostKeyRequest

	secretFn  func(n int, req prompt.SecretRequest) (prompt.SecretAnswer, error)
	hostKeyFn func(req prompt.HostKeyRequest) (prompt.HostKeyAnswer, error)
}

func (p *fakePrompter) AskSecret(_ context.Context, req prompt.SecretRequest) (prompt.SecretAnswer, error) {
	p.mu.Lock()
	p.secrets = append(p.secrets, req)
	n := len(p.secrets) - 1
	fn := p.secretFn
	p.mu.Unlock()
	if fn == nil {
		return prompt.SecretAnswer{Cancel: true}, nil
	}
	return fn(n, req)
}

func (p *fakePrompter) AskHostKey(_ context.Context, req prompt.HostKeyRequest) (prompt.HostKeyAnswer, error) {
	p.mu.Lock()
	p.hostKeys = append(p.hostKeys, req)
	fn := p.hostKeyFn
	p.mu.Unlock()
	if fn == nil {
		return prompt.HostKeyAnswer{}, nil
	}
	return fn(req)
}

func (p *fakePrompter) secretCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.secrets)
}

func (p *fakePrompter) hostKeyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.hostKeys)
}

func answer(v string) func(int, prompt.SecretRequest) (prompt.SecretAnswer, error) {
	return func(int, prompt.SecretRequest) (prompt.SecretAnswer, error) {
		return prompt.SecretAnswer{Value: v}, nil
	}
}

func acceptHostKey(prompt.HostKeyRequest) (prompt.HostKeyAnswer, error) {
	return prompt.HostKeyAnswer{Accept: true}, nil
}

// optsFor builds Options for ep that trust the host via a pre-written known_hosts and
// ignore the developer's agent. Tests override what they exercise.
func optsFor(t *testing.T, ep sshdtest.Endpoint) sshcfg.Options {
	t.Helper()
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(kh, []byte(ep.KnownHostsLine(ep.Host)), 0o600); err != nil {
		t.Fatal(err)
	}
	return sshcfg.Options{
		Alias:                 "test",
		HostName:              ep.Host,
		User:                  ep.User,
		Port:                  ep.Port,
		IdentityAgent:         "none",
		UserKnownHostsFiles:   []string{kh},
		StrictHostKeyChecking: "yes",
	}
}

func dialOK(t *testing.T, d *sshconn.Dialer, o sshcfg.Options) {
	t.Helper()
	c, err := d.Dial(t.Context(), o)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = c.Close()
}

func itoa(n int) string { return strconv.Itoa(n) }
