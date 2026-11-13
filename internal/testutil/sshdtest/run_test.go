package sshdtest

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/w33ble/portunus/internal/testutil"
)

// A port taken by someone else must not look like our sshd being ready: parallel tests pick
// ports that can collide, and a plain dial would succeed against the other listener.
func TestRunRejectsPortHeldByAnotherProcess(t *testing.T) {
	if _, err := os.Stat(sshdPath); err != nil {
		testutil.Unavailable(t, "sshd not available: %v", err)
	}

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	hostKey := NewKey(t).WriteFiles(t, dir, "host", "", false)
	cfg := filepath.Join(dir, "sshd_config")
	body := fmt.Sprintf("ListenAddress 127.0.0.1\nPort %d\nHostKey %s\nPidFile %s\nUsePAM no\nLogLevel INFO\n",
		port, hostKey, filepath.Join(dir, "sshd.pid"))
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	stop, err := run(cfg, filepath.Join(dir, "sshd.log"), port)
	if err == nil {
		stop()
		t.Fatal("run reported ready for a port held by another listener")
	}
}
