package sshdtest_test

import (
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/w33ble/portunus/internal/testutil/sshdtest"
)

func dial(t *testing.T, addr, user string, auth ssh.AuthMethod, hk ssh.PublicKey) error {
	t.Helper()
	c, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.FixedHostKey(hk),
	})
	if err == nil {
		_ = c.Close()
	}
	return err
}

func TestSSHDPublicKeyAuth(t *testing.T) {
	t.Parallel()
	good, bad := sshdtest.NewKey(t), sshdtest.NewKey(t)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{good.Public}})

	if err := dial(t, s.Addr, s.User, ssh.PublicKeys(good.Signer), s.HostKey); err != nil {
		t.Fatalf("authorized key rejected: %v\n%s", err, s.Log())
	}
	if err := dial(t, s.Addr, s.User, ssh.PublicKeys(bad.Signer), s.HostKey); err == nil {
		t.Fatal("unauthorized key accepted")
	}

	s.Authorize(t, bad.Public)
	if err := dial(t, s.Addr, s.User, ssh.PublicKeys(bad.Signer), s.HostKey); err != nil {
		t.Fatalf("key rejected after Authorize: %v", err)
	}
}

func TestInProcPasswordAndKeyboardInteractive(t *testing.T) {
	t.Parallel()
	s := sshdtest.StartInProc(t, sshdtest.InProcOptions{
		Password:            "hunter2",
		KeyboardInteractive: []sshdtest.Question{{Prompt: "Code: ", Echo: true, Answer: "42"}},
	})
	if err := dial(t, s.Addr, s.User, ssh.Password("hunter2"), s.HostKey); err != nil {
		t.Fatalf("password: %v", err)
	}
	if err := dial(t, s.Addr, s.User, ssh.Password("nope"), s.HostKey); err == nil {
		t.Fatal("wrong password accepted")
	}
	ki := ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
		return []string{"42"}, nil
	})
	if err := dial(t, s.Addr, s.User, ki, s.HostKey); err != nil {
		t.Fatalf("keyboard-interactive: %v", err)
	}
}

func TestAgentServesKeys(t *testing.T) {
	t.Parallel()
	k := sshdtest.NewKey(t)
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)
	s := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", a.Sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	signers := agentSigners(t, conn)
	if err := dial(t, s.Addr, s.User, ssh.PublicKeys(signers...), s.HostKey); err != nil {
		t.Fatalf("agent key rejected: %v", err)
	}
}
