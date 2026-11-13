package sshdtest

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Proxy forwards TCP to a backend and can add one-way latency or silently drop traffic,
// simulating slow and dead links.
type Proxy struct {
	Port    int
	Addr    string
	paused  atomic.Bool
	latency atomic.Int64
}

// Pause starts or stops dropping traffic in both directions (connections stay open).
func (p *Proxy) Pause(v bool) { p.paused.Store(v) }

// SetLatency delays every chunk by d in each direction.
func (p *Proxy) SetLatency(d time.Duration) { p.latency.Store(int64(d)) }

// StartProxy listens on a random localhost port and forwards to backend.
func StartProxy(t testing.TB, backend string) *Proxy {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}
	p := &Proxy{Port: ln.Addr().(*net.TCPAddr).Port}
	p.Addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port))

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		conns []net.Conn
	)
	track := func(cs ...net.Conn) {
		mu.Lock()
		conns = append(conns, cs...)
		mu.Unlock()
	}
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			b, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", backend)
			if err != nil {
				_ = c.Close()
				continue
			}
			track(c, b)
			wg.Go(func() { p.pump(&wg, c, b) })
			wg.Go(func() { p.pump(&wg, b, c) })
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
	return p
}

type chunk struct {
	data []byte
	at   time.Time
}

// pump copies src to dst through a delay line so latency doesn't throttle bandwidth.
func (p *Proxy) pump(wg *sync.WaitGroup, src, dst net.Conn) {
	line := make(chan chunk, 4096)
	wg.Go(func() {
		for c := range line {
			time.Sleep(time.Until(c.at))
			if p.paused.Load() {
				continue
			}
			if _, err := dst.Write(c.data); err != nil {
				_ = src.Close()
				for range line { //nolint:revive // drain so the reader never blocks
				}
				return
			}
		}
	})
	defer close(line)
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			line <- chunk{data: append([]byte(nil), buf[:n]...), at: time.Now().Add(time.Duration(p.latency.Load()))}
		}
		if err != nil {
			return
		}
	}
}
