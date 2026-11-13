package sftpfs_test

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/testutil/sshdtest"
	"github.com/w33ble/portunus/internal/vfs/sftpfs"
)

// rig is one sshd plus credentials to open any number of sftpfs connections to it.
type rig struct {
	srv   *sshdtest.Server
	agent *sshdtest.Agent
}

func newRig(t *testing.T) *rig {
	t.Helper()
	k := sshdtest.NewKey(t)
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)
	return &rig{srv: sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}}), agent: a}
}

func (r *rig) options(t *testing.T) sshcfg.Options {
	t.Helper()
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(kh, []byte(r.srv.KnownHostsLine(r.srv.Host)), 0o600); err != nil {
		t.Fatal(err)
	}
	return sshcfg.Options{
		Alias: "test", HostName: r.srv.Host, User: r.srv.User, Port: r.srv.Port,
		IdentityAgent: r.agent.Sock, UserKnownHostsFiles: []string{kh}, StrictHostKeyChecking: "yes",
	}
}

// connect opens a new sftpfs, closed at test cleanup.
func (r *rig) connect(t *testing.T) *sftpfs.FS {
	t.Helper()
	return r.connectOpts(t, r.options(t))
}

func (r *rig) connectOpts(t *testing.T, o sshcfg.Options) *sftpfs.FS {
	t.Helper()
	c, err := (&sshconn.Dialer{}).Dial(t.Context(), o)
	if err != nil {
		t.Fatalf("dial: %v\n%s", err, r.srv.Log())
	}
	f, err := sftpfs.New(c)
	if err != nil {
		_ = c.Close()
		t.Fatalf("sftpfs.New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
