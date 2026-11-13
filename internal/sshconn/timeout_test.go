package sshconn_test

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
)

func TestDialTimesOutOnSilentServer(t *testing.T) {
	t.Parallel()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	o := sshcfg.Options{
		Alias: "silent", HostName: "127.0.0.1", Port: addr.Port, User: "u",
		IdentityAgent: "none", ConnectTimeout: 300 * time.Millisecond,
		UserKnownHostsFiles: []string{os.DevNull},
	}
	start := time.Now()
	_, err = (&sshconn.Dialer{}).Dial(t.Context(), o)
	if err == nil {
		t.Fatal("Dial succeeded against a silent server")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Dial took %v with a 300ms ConnectTimeout", d)
	}
}
