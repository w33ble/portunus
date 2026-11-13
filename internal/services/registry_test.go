package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/localfs"
)

// stubFS is a vfs.FS that only knows its home and whether it was closed.
type stubFS struct {
	vfs.FS
	closed atomic.Bool
}

func (s *stubFS) Kind() vfs.Kind                       { return vfs.KindSFTP }
func (s *stubFS) Home(context.Context) (string, error) { return "/home/u", nil }
func (s *stubFS) Close() error                         { s.closed.Store(true); return nil }

type fakeDialer struct {
	mu    sync.Mutex
	calls int
	gate  chan struct{} // when non-nil, dials block until closed
	err   error
	lost  chan struct{}
	last  *stubFS
}

func (d *fakeDialer) dial(ctx context.Context, t Target) (dialed, error) {
	d.mu.Lock()
	d.calls++
	gate, err := d.gate, d.err
	d.mu.Unlock()

	if gate != nil {
		<-gate
	}
	if err != nil {
		return dialed{}, err
	}

	fs := &stubFS{}
	lost := make(chan struct{})
	d.mu.Lock()
	d.last, d.lost = fs, lost
	d.mu.Unlock()
	return dialed{fs: fs, label: t.Alias, lost: lost}, nil
}

func (d *fakeDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func newTestRegistry(d *fakeDialer) (*registry, *[][]Connection) {
	var mu sync.Mutex
	changes := [][]Connection{}
	r := newRegistry(localfs.New(), d.dial, func(c []Connection) {
		mu.Lock()
		changes = append(changes, c)
		mu.Unlock()
	})
	return r, &changes
}

var hostA = Target{Kind: TargetSSH, Alias: "a"}

func TestTargetIDs(t *testing.T) {
	cases := []struct {
		t    Target
		want string
	}{
		{Target{Kind: TargetLocal}, "local"},
		{Target{Kind: TargetSSH, Alias: "web"}, "ssh:web"},
		{Target{Kind: TargetSSH, Host: "h"}, "ssh:h"},
		{Target{Kind: TargetSSH, User: "u", Host: "h", Port: 2222}, "ssh:u@h:2222"},
		{Target{Kind: TargetSSH, Host: "::1", Port: 22}, "ssh:[::1]:22"},
		{Target{Kind: TargetS3, ProfileID: "p1"}, "s3:p1"},
	}
	for _, c := range cases {
		got, err := targetID(c.t)
		if err != nil || got != c.want {
			t.Errorf("targetID(%+v) = %q, %v; want %q", c.t, got, err, c.want)
		}
	}

	for _, bad := range []Target{{Kind: TargetSSH}, {Kind: TargetS3}, {Kind: "ftp"}} {
		if _, err := targetID(bad); err == nil {
			t.Errorf("targetID(%+v) accepted", bad)
		}
	}
}

func TestLocalIsAlwaysPresentAndFirst(t *testing.T) {
	r, _ := newTestRegistry(&fakeDialer{})
	if _, err := r.Connect(context.Background(), Target{Kind: TargetSSH, Alias: "zzz"}); err != nil {
		t.Fatal(err)
	}

	list := r.List()
	if len(list) != 2 || list[0].ID != LocalID {
		t.Fatalf("list = %+v, want local first", list)
	}
	if err := r.Disconnect(LocalID); err == nil {
		t.Fatal("disconnecting local must fail")
	}
}

func TestConcurrentConnectsShareOneDial(t *testing.T) {
	d := &fakeDialer{gate: make(chan struct{})}
	r, _ := newTestRegistry(d)

	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Go(func() {
			c, err := r.Connect(context.Background(), hostA)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = c.ID
		})
	}

	// Let every goroutine reach the single-flight group before the dial completes.
	time.Sleep(50 * time.Millisecond)
	close(d.gate)
	wg.Wait()

	if n := d.count(); n != 1 {
		t.Fatalf("dials = %d, want 1", n)
	}
	for _, id := range ids {
		if id != "ssh:a" {
			t.Fatalf("ids = %v", ids)
		}
	}
}

func TestCanceledCallerDoesNotFailSharedDial(t *testing.T) {
	d := &fakeDialer{gate: make(chan struct{})}
	r, _ := newTestRegistry(d)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := r.Connect(ctx, hostA)
		errc <- err
	}()

	other := make(chan error, 1)
	go func() {
		_, err := r.Connect(context.Background(), hostA)
		other <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller err = %v", err)
	}

	close(d.gate)
	if err := <-other; err != nil {
		t.Fatalf("other caller err = %v, want shared dial to succeed", err)
	}
}

func TestLostConnectionRedialsOnNextUse(t *testing.T) {
	d := &fakeDialer{}
	r, _ := newTestRegistry(d)

	if _, err := r.Connect(context.Background(), hostA); err != nil {
		t.Fatal(err)
	}
	first := d.last
	close(d.lost)

	waitFor(t, func() bool { return len(r.List()) == 1 })
	if !first.closed.Load() {
		t.Fatal("lost connection's FS was not closed")
	}

	fs, err := r.FS(context.Background(), "ssh:a")
	if err != nil {
		t.Fatal(err)
	}
	if fs == first || d.count() != 2 {
		t.Fatalf("expected a fresh dial, calls = %d", d.count())
	}
}

func TestDisconnectForgetsTarget(t *testing.T) {
	d := &fakeDialer{}
	r, changes := newTestRegistry(d)

	if _, err := r.Connect(context.Background(), hostA); err != nil {
		t.Fatal(err)
	}
	if err := r.Disconnect("ssh:a"); err != nil {
		t.Fatal(err)
	}
	if !d.last.closed.Load() {
		t.Fatal("disconnect did not close the FS")
	}
	if _, err := r.FS(context.Background(), "ssh:a"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("FS after disconnect err = %v, want ErrNotConnected", err)
	}
	if len(*changes) != 2 {
		t.Fatalf("change notifications = %d, want 2 (connect, disconnect)", len(*changes))
	}
}

func TestDialErrorIsReturnedAndNotCached(t *testing.T) {
	d := &fakeDialer{err: errors.New("auth failed")}
	r, _ := newTestRegistry(d)

	if _, err := r.Connect(context.Background(), hostA); err == nil {
		t.Fatal("dial error swallowed")
	}

	d.mu.Lock()
	d.err = nil
	d.mu.Unlock()
	if _, err := r.Connect(context.Background(), hostA); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
