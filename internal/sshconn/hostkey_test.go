package sshconn_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/testutil/sshdtest"
)

// unknownHostSetup starts a server, an agent holding its key, and Options with an empty
// (not yet created) known_hosts, so the host key is unknown.
func unknownHostSetup(t *testing.T) (*sshdtest.Server, sshcfg.Options, string) {
	t.Helper()
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	return s, sshcfg.Options{
		Alias:               "myhost",
		HostName:            s.Host,
		User:                s.User,
		Port:                s.Port,
		IdentityAgent:       a.Sock,
		UserKnownHostsFiles: []string{kh},
	}, kh
}

func TestTOFUAcceptWritesKnownHosts(t *testing.T) {
	t.Parallel()
	s, o, kh := unknownHostSetup(t)
	p := &fakePrompter{hostKeyFn: acceptHostKey}
	d := &sshconn.Dialer{Prompter: p}

	dialOK(t, d, o)
	if p.hostKeyCount() != 1 {
		t.Fatalf("host key prompts = %d", p.hostKeyCount())
	}
	req := p.hostKeys[0]
	if req.Host != "myhost" || req.SaveTo != kh || req.KeyType != "ssh-ed25519" ||
		req.Fingerprint != ssh.FingerprintSHA256(s.HostKey) || !strings.HasSuffix(req.Address, s.Addr) {
		t.Fatalf("unexpected host key request %+v", req)
	}
	got, err := os.ReadFile(kh)
	if err != nil || string(got) != s.KnownHostsLine(s.Host) {
		t.Fatalf("known_hosts = %q (%v), want %q", got, err, s.KnownHostsLine(s.Host))
	}
	if fi, _ := os.Stat(kh); fi.Mode().Perm() != 0o600 {
		t.Fatalf("known_hosts mode = %v", fi.Mode().Perm())
	}

	dialOK(t, d, o)
	if p.hostKeyCount() != 1 {
		t.Fatalf("known host prompted again (%d)", p.hostKeyCount())
	}
}

func TestTOFUAppendsToExistingFileWithoutTrailingNewline(t *testing.T) {
	t.Parallel()
	_, o, kh := unknownHostSetup(t)
	first := "other.example " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshdtest.NewKey(t).Public)))
	if err := os.WriteFile(kh, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	dialOK(t, &sshconn.Dialer{Prompter: &fakePrompter{hostKeyFn: acceptHostKey}}, o)
	b, _ := os.ReadFile(kh)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || lines[0] != first {
		t.Fatalf("known_hosts corrupted: %q", b)
	}
}

func TestTOFURejectFailsAndWritesNothing(t *testing.T) {
	t.Parallel()
	_, o, kh := unknownHostSetup(t)
	p := &fakePrompter{} // nil hostKeyFn rejects
	_, err := (&sshconn.Dialer{Prompter: p}).Dial(t.Context(), o)
	if !errors.Is(err, sshconn.ErrHostKeyRejected) {
		t.Fatalf("err = %v, want ErrHostKeyRejected", err)
	}
	if _, err := os.Stat(kh); err == nil {
		t.Fatal("rejected key was written")
	}
}

func TestTOFUHashedKnownHosts(t *testing.T) {
	t.Parallel()
	s, o, kh := unknownHostSetup(t)
	o.HashKnownHosts = true
	p := &fakePrompter{hostKeyFn: acceptHostKey}
	d := &sshconn.Dialer{Prompter: p}
	dialOK(t, d, o)

	b, _ := os.ReadFile(kh)
	if !strings.HasPrefix(string(b), "|1|") || strings.Contains(string(b), s.Host) {
		t.Fatalf("line not hashed: %q", b)
	}
	dialOK(t, d, o)
	if p.hostKeyCount() != 1 {
		t.Fatal("hashed entry not recognised on redial")
	}
}

func TestTOFUFallsBackToAppKnownHosts(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission checks don't apply to root")
	}
	_, o, _ := unknownHostSetup(t)
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) }) //nolint:gosec // restore so TempDir cleanup works
	o.UserKnownHostsFiles = []string{filepath.Join(ro, "known_hosts")}
	app := filepath.Join(t.TempDir(), "config", "Portunus", "known_hosts")

	p := &fakePrompter{hostKeyFn: acceptHostKey}
	d := &sshconn.Dialer{Prompter: p, AppKnownHosts: app}
	dialOK(t, d, o)
	if p.hostKeys[0].SaveTo != app {
		t.Fatalf("prompt SaveTo = %q, want %q", p.hostKeys[0].SaveTo, app)
	}
	if b, err := os.ReadFile(app); err != nil || len(b) == 0 {
		t.Fatalf("app known_hosts empty: %v", err)
	}
	if fi, _ := os.Stat(filepath.Dir(app)); fi.Mode().Perm() != 0o700 {
		t.Fatalf("app dir mode = %v", fi.Mode().Perm())
	}
	dialOK(t, d, o)
	if p.hostKeyCount() != 1 {
		t.Fatal("app known_hosts not consulted on redial")
	}
}

func TestTOFUMissingUserDirFallsBackToApp(t *testing.T) {
	t.Parallel()
	_, o, _ := unknownHostSetup(t)
	o.UserKnownHostsFiles = []string{filepath.Join(t.TempDir(), "no-such-dir", "known_hosts")}
	app := filepath.Join(t.TempDir(), "app", "known_hosts")
	p := &fakePrompter{hostKeyFn: acceptHostKey}
	dialOK(t, &sshconn.Dialer{Prompter: p, AppKnownHosts: app}, o)
	if p.hostKeys[0].SaveTo != app {
		t.Fatalf("SaveTo = %q, want %q", p.hostKeys[0].SaveTo, app)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(o.UserKnownHostsFiles[0]))); err == nil {
		t.Fatal("user ~/.ssh-like directory was created")
	}
}

func TestStrictHostKeyCheckingModes(t *testing.T) {
	t.Parallel()
	t.Run("yes errors without prompting", func(t *testing.T) {
		t.Parallel()
		_, o, kh := unknownHostSetup(t)
		o.StrictHostKeyChecking = "yes"
		p := &fakePrompter{hostKeyFn: acceptHostKey}
		_, err := (&sshconn.Dialer{Prompter: p}).Dial(t.Context(), o)
		var unk *sshconn.HostKeyUnknownError
		if !errors.As(err, &unk) || p.hostKeyCount() != 0 {
			t.Fatalf("err=%v prompts=%d", err, p.hostKeyCount())
		}
		if _, err := os.Stat(kh); err == nil {
			t.Fatal("known_hosts written under StrictHostKeyChecking=yes")
		}
	})
	for _, mode := range []string{"no", "off", "accept-new"} {
		t.Run(mode+" adds without prompting", func(t *testing.T) {
			t.Parallel()
			_, o, kh := unknownHostSetup(t)
			o.StrictHostKeyChecking = mode
			p := &fakePrompter{}
			dialOK(t, &sshconn.Dialer{Prompter: p}, o)
			if p.hostKeyCount() != 0 {
				t.Fatal("prompted")
			}
			if b, _ := os.ReadFile(kh); len(b) == 0 {
				t.Fatal("key not recorded")
			}
		})
	}
	t.Run("no prompter errors", func(t *testing.T) {
		t.Parallel()
		_, o, _ := unknownHostSetup(t)
		_, err := (&sshconn.Dialer{}).Dial(t.Context(), o)
		if _, ok := errors.AsType[*sshconn.HostKeyUnknownError](err); !ok {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestHostKeyMismatchNeverPrompts(t *testing.T) {
	t.Parallel()
	s, o, kh := unknownHostSetup(t)
	other := sshdtest.NewKey(t)
	line := "[" + s.Host + "]:" + itoa(s.Port) + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(other.Public))) + "\n"
	if err := os.WriteFile(kh, []byte("# comment\n"+line), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &fakePrompter{hostKeyFn: acceptHostKey}
	_, err := (&sshconn.Dialer{Prompter: p}).Dial(t.Context(), o)
	var mm *sshconn.HostKeyMismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("err = %v, want HostKeyMismatchError", err)
	}
	if mm.File != kh || mm.Line != 2 || mm.Fingerprint != ssh.FingerprintSHA256(s.HostKey) || mm.KeyType != "ssh-ed25519" {
		t.Fatalf("mismatch details = %+v", mm)
	}
	if p.hostKeyCount() != 0 || p.secretCount() != 0 {
		t.Fatal("prompted on mismatch")
	}
}

func TestKnownHostsFromGlobalAndAppFiles(t *testing.T) {
	t.Parallel()
	s, o, _ := unknownHostSetup(t)
	o.StrictHostKeyChecking = "yes"
	dir := t.TempDir()

	global := filepath.Join(dir, "global")
	if err := os.WriteFile(global, []byte(s.KnownHostsLine(s.Host)), 0o600); err != nil {
		t.Fatal(err)
	}
	o.GlobalKnownHostsFiles = []string{global, filepath.Join(dir, "missing")}
	dialOK(t, &sshconn.Dialer{}, o)

	o.GlobalKnownHostsFiles = nil
	app := filepath.Join(dir, "app")
	if err := os.WriteFile(app, []byte(s.KnownHostsLine(s.Host)), 0o600); err != nil {
		t.Fatal(err)
	}
	dialOK(t, &sshconn.Dialer{AppKnownHosts: app}, o)
}

func TestHostKeyAliasIsRecordedAndLookedUp(t *testing.T) {
	t.Parallel()
	s, o, kh := unknownHostSetup(t)
	o.HostKeyAlias = "prod-db"
	p := &fakePrompter{hostKeyFn: acceptHostKey}
	d := &sshconn.Dialer{Prompter: p}
	dialOK(t, d, o)

	// OpenSSH records the alias bare, regardless of port.
	want := "prod-db " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.HostKey))) + "\n"
	if b, _ := os.ReadFile(kh); string(b) != want {
		t.Fatalf("known_hosts = %q, want %q", b, want)
	}
	dialOK(t, d, o)
	if p.hostKeyCount() != 1 {
		t.Fatal("alias entry not found on second dial")
	}
}

func TestHostKeyAlgorithmFollowsKnownKey(t *testing.T) {
	t.Parallel()
	// The server holds ed25519 and rsa keys but known_hosts only the ed25519 one. Unless the
	// client asks for the known key's algorithm it may negotiate ed25519 and see a mismatch.
	k := sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}, RSAHostKey: true})
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)
	o := optsFor(t, s.Endpoint)
	o.IdentityAgent = a.Sock
	line := "[" + s.Host + "]:" + itoa(s.Port) + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.HostKey))) + "\n"
	if err := os.WriteFile(o.UserKnownHostsFiles[0], []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	dialOK(t, &sshconn.Dialer{}, o)
}
