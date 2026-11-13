package transfer

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/testutil/sshdtest"
	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/localfs"
	"github.com/w33ble/portunus/internal/vfs/sftpfs"
)

// slowLink starts sshd behind a 50ms-RTT proxy and returns a function opening new sftpfs
// connections through it.
func slowLink(t *testing.T) func() *sftpfs.FS {
	t.Helper()
	k := sshdtest.NewKey(t)
	a := sshdtest.StartAgent(t)
	a.AddKey(t, k)
	srv := sshdtest.Start(t, sshdtest.Options{AuthorizedKeys: []ssh.PublicKey{k.Public}})
	px := sshdtest.StartProxy(t, srv.Addr)
	px.SetLatency(25 * time.Millisecond)
	return func() *sftpfs.FS {
		o := sshcfg.Options{
			Alias: "test", HostName: srv.Host, User: srv.User, Port: px.Port,
			IdentityAgent: a.Sock, StrictHostKeyChecking: "no",
			UserKnownHostsFiles: []string{filepath.Join(t.TempDir(), "kh")},
		}
		c, err := (&sshconn.Dialer{}).Dial(t.Context(), o)
		if err != nil {
			t.Fatalf("dial: %v\n%s", err, srv.Log())
		}
		f, err := sftpfs.New(c)
		if err != nil {
			_ = c.Close()
			t.Fatalf("sftpfs.New: %v", err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
}

func TestCopyThroughputOverHighLatencyLink(t *testing.T) {
	t.Parallel()
	skipUnderRace(t)
	const size = 32 << 20
	connect := slowLink(t)
	local := localfs.New()
	remoteA, remoteB := connect(), connect()

	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	srcDir, up, mid := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "big.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	conns := map[string]vfs.FS{"local": local, "a": remoteA, "b": remoteB}
	h := newHarness(t, func(o *Options) {
		o.Resolve = func(id string) (vfs.FS, error) { return conns[id], nil }
	})

	upMin, hopMin := 10.0, 5.0
	run := func(name, srcConn, src, dstConn, dstDir string, minMBps float64) {
		start := time.Now()
		id := h.copyJob(srcConn, []string{src}, dstConn, dstDir, PolicyAsk)
		v := h.waitFinal(id)
		if v.State != StateDone {
			t.Fatalf("%s: state %s: %+v", name, v.State, v)
		}
		mbps := float64(size) / time.Since(start).Seconds() / 1e6
		t.Logf("%s: %.1f MB/s", name, mbps)
		if mbps < minMBps {
			t.Errorf("%s: %.1f MB/s, want >= %.0f", name, mbps, minMBps)
		}
	}

	run("local->sftp", "local", filepath.Join(srcDir, "big.bin"), "a", up, upMin)
	got, err := os.ReadFile(filepath.Join(up, "big.bin"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("uploaded content differs (err %v)", err)
	}
	run("sftp->sftp", "a", filepath.Join(up, "big.bin"), "b", mid, hopMin)
	f, err := os.Open(filepath.Join(mid, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got, err = io.ReadAll(f)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("sftp->sftp content differs (err %v)", err)
	}
}
