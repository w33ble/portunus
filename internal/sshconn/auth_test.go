package sshconn_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/testutil"
	"github.com/w33ble/portunus/internal/testutil/sshdtest"
)

func TestAgentKey(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)

	p := &fakePrompter{}
	o := optsFor(t, s.Endpoint)
	o.IdentityAgent = a.Sock
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 0 {
		t.Fatalf("agent auth prompted %d times", p.secretCount())
	}
}

func TestAgentFromSSHAuthSock(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)

	o := optsFor(t, s.Endpoint)
	o.IdentityAgent = ""
	getenv := func(key string) string {
		if key == "SSH_AUTH_SOCK" {
			return a.Sock
		}
		return ""
	}
	dialOK(t, &sshconn.Dialer{Getenv: getenv}, o)

	o.IdentityAgent = "none"
	if _, err := (&sshconn.Dialer{Getenv: getenv}).Dial(t.Context(), o); err == nil {
		t.Fatal("IdentityAgent none still used the agent")
	}
}

func TestPubOnlyIdentityFileSelectsAgentKey(t *testing.T) {
	t.Parallel()
	good, other := sshdtest.NewKey(t), sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{good.Public}})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, other)
	a.AddKey(t, good)
	dir := t.TempDir()

	o := optsFor(t, s.Endpoint)
	o.IdentityAgent = a.Sock
	o.IdentitiesOnly = true

	o.IdentityFiles = []string{good.WritePub(t, dir, "good")}
	dialOK(t, &sshconn.Dialer{}, o)

	// A selector for a key the server doesn't accept must not fall back to other agent keys.
	o.IdentityFiles = []string{other.WritePub(t, dir, "other")}
	if _, err := (&sshconn.Dialer{}).Dial(t.Context(), o); err == nil {
		t.Fatal("IdentitiesOnly offered an agent key that no IdentityFile selects")
	}
}

func TestIdentitiesOnlyFiltersAgent(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)

	o := optsFor(t, s.Endpoint)
	o.IdentityAgent = a.Sock
	dialOK(t, &sshconn.Dialer{}, o)

	o.IdentitiesOnly = true
	if _, err := (&sshconn.Dialer{}).Dial(t.Context(), o); err == nil {
		t.Fatal("IdentitiesOnly with no matching IdentityFile still used the agent key")
	}
}

func TestKeyFiles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		key  func(testing.TB) sshdtest.Key
		pass string
		pub  bool
	}{
		{"unencrypted", sshdtest.NewKey, "", false},
		{"unencrypted with pub", sshdtest.NewKey, "", true},
		{"encrypted with pub", sshdtest.NewKey, "s3cret", true},
		{"encrypted without pub", sshdtest.NewKey, "s3cret", false},
		{"encrypted rsa", func(t testing.TB) sshdtest.Key { return sshdtest.NewRSAKey(t, 2048) }, "s3cret", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			k := tc.key(t)
			s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
			o := optsFor(t, s.Endpoint)
			o.IdentityFiles = []string{k.WriteFiles(t, t.TempDir(), "id", tc.pass, tc.pub)}

			p := &fakePrompter{secretFn: answer(tc.pass)}
			dialOK(t, &sshconn.Dialer{Prompter: p}, o)

			want := 0
			if tc.pass != "" {
				want = 1
			}
			if got := p.secretCount(); got != want {
				t.Fatalf("passphrase prompts = %d, want %d", got, want)
			}
			if want == 1 {
				req := p.secrets[0]
				if !strings.HasPrefix(req.Title, "Passphrase for ") || !strings.HasSuffix(req.Title, "id") || req.Echo {
					t.Fatalf("unexpected prompt %+v", req)
				}
			}
		})
	}
}

func TestPassphraseOnlyPromptedWhenServerAcceptsKey(t *testing.T) {
	t.Parallel()
	encrypted, agentKey := sshdtest.NewKey(t), sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{agentKey.Public}})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, agentKey)

	o := optsFor(t, s.Endpoint)
	o.IdentityAgent = a.Sock
	// The encrypted file comes first but the server doesn't accept it.
	o.IdentityFiles = []string{encrypted.WriteFiles(t, t.TempDir(), "enc", "pw", true)}

	p := &fakePrompter{secretFn: answer("pw")}
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 0 {
		t.Fatalf("prompted for a passphrase although the server rejected that key (%d prompts)", p.secretCount())
	}

	s.Authorize(t, encrypted.Public)
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 1 {
		t.Fatalf("prompts after server accepts the key = %d, want 1", p.secretCount())
	}
}

func TestPassphraseRetryAndCancel(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	o := optsFor(t, s.Endpoint)
	o.IdentityFiles = []string{k.WriteFiles(t, t.TempDir(), "id", "right", true)}

	p := &fakePrompter{secretFn: func(n int, _ prompt.SecretRequest) (prompt.SecretAnswer, error) {
		if n == 0 {
			return prompt.SecretAnswer{Value: "wrong"}, nil
		}
		return prompt.SecretAnswer{Value: "right"}, nil
	}}
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 2 || p.secrets[1].Message == "" {
		t.Fatalf("want a second prompt carrying an error message, got %+v", p.secrets)
	}

	always := &fakePrompter{secretFn: answer("nope")}
	_, err := (&sshconn.Dialer{Prompter: always}).Dial(t.Context(), o)
	if !errors.Is(err, sshconn.ErrBadPassphrase) || always.secretCount() != 3 {
		t.Fatalf("bad passphrases: err=%v prompts=%d", err, always.secretCount())
	}

	cancelled := &fakePrompter{} // nil secretFn cancels
	_, err = (&sshconn.Dialer{Prompter: cancelled}).Dial(t.Context(), o)
	if !errors.Is(err, sshconn.ErrCanceled) || cancelled.secretCount() != 1 {
		t.Fatalf("cancel: err=%v prompts=%d", err, cancelled.secretCount())
	}
}

func TestContextCancelDuringPrompt(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	o := optsFor(t, s.Endpoint)
	o.IdentityFiles = []string{k.WriteFiles(t, t.TempDir(), "id", "pw", true)}

	ctx, cancel := context.WithCancel(t.Context())
	p := &fakePrompter{secretFn: func(int, prompt.SecretRequest) (prompt.SecretAnswer, error) {
		cancel()
		<-ctx.Done()
		return prompt.SecretAnswer{}, ctx.Err()
	}}
	_, err := (&sshconn.Dialer{Prompter: p}).Dial(ctx, o)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRedialAfterLoginGraceTimeoutKeepsPassphrase(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{
		AuthorizedKeys: []ssh.PublicKey{k.Public},
		ExtraConfig:    "LoginGraceTime 1",
	})
	o := optsFor(t, s.Endpoint)
	o.IdentityFiles = []string{k.WriteFiles(t, t.TempDir(), "id", "pw", true)}

	p := &fakePrompter{secretFn: func(int, prompt.SecretRequest) (prompt.SecretAnswer, error) {
		time.Sleep(1500 * time.Millisecond) // longer than the server's grace time
		return prompt.SecretAnswer{Value: "pw"}, nil
	}}
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 1 {
		t.Fatalf("prompts = %d, want 1 (redial must reuse the decrypted key)", p.secretCount())
	}
}

func TestPasswordRetry(t *testing.T) {
	t.Parallel()
	s := sshdtest.StartInProc(t, sshdtest.InProcOptions{Password: "hunter2"})
	o := optsFor(t, s.Endpoint)
	p := &fakePrompter{secretFn: func(n int, _ prompt.SecretRequest) (prompt.SecretAnswer, error) {
		if n == 0 {
			return prompt.SecretAnswer{Value: "bad"}, nil
		}
		return prompt.SecretAnswer{Value: "hunter2"}, nil
	}}
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 2 || p.secrets[0].Label != "Password" || p.secrets[0].Echo || p.secrets[1].Message == "" {
		t.Fatalf("prompts = %+v", p.secrets)
	}
}

func TestKeyboardInteractiveQuestions(t *testing.T) {
	t.Parallel()
	s := sshdtest.StartInProc(t, sshdtest.InProcOptions{KeyboardInteractive: []sshdtest.Question{
		{Prompt: "Verification code: ", Echo: true, Answer: "123456"},
		{Prompt: "Password: ", Echo: false, Answer: "pw"},
	}})
	o := optsFor(t, s.Endpoint)
	p := &fakePrompter{secretFn: func(_ int, r prompt.SecretRequest) (prompt.SecretAnswer, error) {
		if r.Echo {
			return prompt.SecretAnswer{Value: "123456"}, nil
		}
		return prompt.SecretAnswer{Value: "pw"}, nil
	}}
	dialOK(t, &sshconn.Dialer{Prompter: p}, o)
	if p.secretCount() != 2 {
		t.Fatalf("prompts = %+v", p.secrets)
	}
	if r := p.secrets[0]; r.Label != "Verification code:" || !r.Echo || r.Message != "test instruction" {
		t.Fatalf("first question = %+v", r)
	}
	if r := p.secrets[1]; r.Echo {
		t.Fatalf("second question echoes: %+v", r)
	}
}

func TestKeyboardInteractiveBeforePassword(t *testing.T) {
	t.Parallel()
	s := sshdtest.StartInProc(t, sshdtest.InProcOptions{
		Password:            "pw",
		KeyboardInteractive: []sshdtest.Question{{Prompt: "Code: ", Answer: "1"}},
	})
	p := &fakePrompter{secretFn: answer("1")}
	dialOK(t, &sshconn.Dialer{Prompter: p}, optsFor(t, s.Endpoint))
	if got := s.Attempts(); len(got) == 0 || got[0] != "keyboard-interactive" {
		t.Fatalf("attempt order = %v, want keyboard-interactive first", got)
	}
}

func TestCancelStopsAllPrompting(t *testing.T) {
	t.Parallel()
	s := sshdtest.StartInProc(t, sshdtest.InProcOptions{
		Password:            "pw",
		KeyboardInteractive: []sshdtest.Question{{Prompt: "Code: ", Answer: "1"}},
	})
	p := &fakePrompter{}
	_, err := (&sshconn.Dialer{Prompter: p}).Dial(t.Context(), optsFor(t, s.Endpoint))
	if !errors.Is(err, sshconn.ErrCanceled) || p.secretCount() != 1 {
		t.Fatalf("err=%v prompts=%d", err, p.secretCount())
	}
}

func TestNoPrompterFailsPasswordAuth(t *testing.T) {
	t.Parallel()
	s := sshdtest.StartInProc(t, sshdtest.InProcOptions{Password: "pw"})
	if _, err := (&sshconn.Dialer{}).Dial(t.Context(), optsFor(t, s.Endpoint)); err == nil {
		t.Fatal("dial succeeded without credentials")
	}
}

func TestCertificateFile(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		testutil.Unavailable(t, "ssh-keygen not available")
	}
	ca, user := sshdtest.NewKey(t), sshdtest.NewKey(t)
	dir := t.TempDir()
	caPath := ca.WriteFiles(t, dir, "ca", "", true)
	idPath := user.WriteFiles(t, dir, "id", "", true)

	s := sshdtest.Start(t, sshdtest.Options{ExtraConfig: "TrustedUserCAKeys " + caPath + ".pub"})
	cmd := exec.CommandContext(t.Context(), "ssh-keygen", "-q", "-s", caPath, "-I", "test", "-n", s.User, idPath+".pub")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen -s: %v\n%s", err, out)
	}

	o := optsFor(t, s.Endpoint)
	o.IdentityFiles = []string{idPath}
	if _, err := (&sshconn.Dialer{}).Dial(t.Context(), o); err == nil {
		t.Fatal("plain key accepted although only the CA is trusted")
	}
	o.CertificateFiles = []string{idPath + "-cert.pub"}
	dialOK(t, &sshconn.Dialer{}, o)
}

func TestProxyCommandUnsupported(t *testing.T) {
	t.Parallel()
	_, err := (&sshconn.Dialer{}).Dial(t.Context(), sshcfg.Options{HostName: "x", ProxyCommand: "nc %h %p"})
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestProxyJump(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	authz := sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}}
	jump1, jump2, target := sshdtest.Start(t, authz), sshdtest.Start(t, authz), sshdtest.Start(t, authz)
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)

	all := map[string]sshdtest.Endpoint{"jump1": jump1.Endpoint, "jump2": jump2.Endpoint}
	d := &sshconn.Dialer{
		Resolve: func(_ context.Context, alias string) (sshcfg.Options, error) {
			ep, ok := all[alias]
			if alias == "jumpwrong" {
				ep, ok = jump1.Endpoint, true
			}
			if !ok {
				return sshcfg.Options{}, errors.New("unknown alias " + alias)
			}
			o := optsFor(t, ep)
			o.Alias, o.IdentityAgent = alias, a.Sock
			if alias == "jumpwrong" {
				o.User, o.Port = "wrong", 1
			}
			return o, nil
		},
	}

	for _, tc := range []struct{ name, jump string }{
		{"one hop", "jump1"},
		{"two hops", "jump1,jump2"},
		{"user and port override config", fmt.Sprintf("%s@jumpwrong:%d", jump1.User, jump1.Port)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := optsFor(t, target.Endpoint)
			o.IdentityAgent = a.Sock
			o.ProxyJump = tc.jump
			c, err := d.Dial(t.Context(), o)
			if err != nil {
				t.Fatalf("Dial via %s: %v", tc.jump, err)
			}
			_ = c.Close()
		})
	}

	t.Run("unknown hop", func(t *testing.T) {
		o := optsFor(t, target.Endpoint)
		o.ProxyJump = "nope"
		if _, err := d.Dial(t.Context(), o); err == nil {
			t.Fatal("expected error for unresolvable hop")
		}
	})
}

func TestKeepAliveClosesDeadConnection(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	px := sshdtest.StartProxy(t, s.Addr)
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)

	o := optsFor(t, s.Endpoint)
	o.Port = px.Port
	o.StrictHostKeyChecking = "no"
	o.UserKnownHostsFiles = []string{filepath.Join(t.TempDir(), "kh")}
	o.IdentityAgent = a.Sock
	o.ServerAliveInterval = 100 * time.Millisecond
	o.ServerAliveCountMax = 2

	c, err := (&sshconn.Dialer{}).Dial(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	done := make(chan struct{})
	go func() { _ = c.Wait(); close(done) }()
	select {
	case <-done:
		t.Fatal("connection closed while the link was healthy")
	case <-time.After(400 * time.Millisecond):
	}
	px.Pause(true)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dead link was not detected by keepalive")
	}
}
