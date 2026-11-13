// Package sshconn dials SSH connections the way the OpenSSH client would: agent and key
// file auth through one publickey method, keyboard-interactive and password fallbacks,
// known_hosts verification with trust-on-first-use prompts, and ProxyJump chains.
package sshconn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/vfs"
)

const (
	defaultKeepAlive    = 30 * time.Second
	defaultKeepAliveMax = 3
	defaultConnTimeout  = 30 * time.Second
	maxJumpDepth        = 8
)

// Prompter is the slice of prompt.Broker the dialer needs.
type Prompter interface {
	AskHostKey(ctx context.Context, req prompt.HostKeyRequest) (prompt.HostKeyAnswer, error)
	AskSecret(ctx context.Context, req prompt.SecretRequest) (prompt.SecretAnswer, error)
}

// Dialer opens authenticated SSH clients. The zero value works (no prompts, no hops).
type Dialer struct {
	// Resolve resolves ProxyJump hop aliases; nil treats hops as bare hosts.
	Resolve  func(ctx context.Context, alias string) (sshcfg.Options, error)
	Prompter Prompter
	// AppKnownHosts is the fallback known_hosts file, written when no user file is writable.
	AppKnownHosts string
	// LoadAppleKeychain runs `/usr/bin/ssh-add --apple-load-keychain` once before the
	// first passphrase prompt (darwin only).
	LoadAppleKeychain bool
	Getenv            func(string) string // default os.Getenv
	Logger            *slog.Logger
}

func (d *Dialer) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.New(slog.DiscardHandler)
}

func (d *Dialer) getenv(k string) string {
	if d.Getenv != nil {
		return d.Getenv(k)
	}
	return os.Getenv(k)
}

// Dial connects to o, following ProxyJump, and starts keepalives on every hop.
// Closing the returned client closes the jump connections behind it.
func (d *Dialer) Dial(ctx context.Context, o sshcfg.Options) (*ssh.Client, error) {
	return d.dial(ctx, o, 0)
}

func (d *Dialer) dial(ctx context.Context, o sshcfg.Options, depth int) (*ssh.Client, error) {
	if o.ProxyCommand != "" {
		return nil, fmt.Errorf("sshconn: ProxyCommand is not supported: %w", vfs.ErrUnsupported)
	}
	if depth > maxJumpDepth {
		return nil, errors.New("sshconn: ProxyJump chain too deep")
	}
	st := newDialState(d, ctx, o)

	via, open, err := d.transport(ctx, st, depth)
	if err != nil {
		return nil, err
	}
	client, err := st.connect(open)
	if err != nil {
		if via != nil {
			_ = via.Close()
		}
		return nil, err
	}
	if via != nil {
		go func() {
			_ = client.Wait()
			_ = via.Close()
		}()
	}
	keepAlive(client, o.ServerAliveInterval, o.ServerAliveCountMax)
	return client, nil
}

type openFunc func(ctx context.Context) (net.Conn, error)

// transport returns the client to tunnel through (nil when direct) and a function
// opening a fresh byte stream to the target.
func (d *Dialer) transport(ctx context.Context, st *dialState, depth int) (*ssh.Client, openFunc, error) {
	target := net.JoinHostPort(st.host(), strconv.Itoa(st.port()))
	hops := splitJump(st.o.ProxyJump)
	if len(hops) == 0 {
		timeout := st.o.ConnectTimeout
		if timeout <= 0 {
			timeout = defaultConnTimeout
		}
		return nil, func(ctx context.Context) (net.Conn, error) {
			nd := net.Dialer{Timeout: timeout}
			c, err := nd.DialContext(ctx, "tcp", target)
			if err != nil {
				return nil, fmt.Errorf("connect %s: %w", target, err)
			}
			return c, nil
		}, nil
	}

	ho, err := d.resolveHop(ctx, hops[len(hops)-1])
	if err != nil {
		return nil, nil, err
	}
	if len(hops) > 1 {
		ho.ProxyJump = strings.Join(hops[:len(hops)-1], ",")
	}
	via, err := d.dial(ctx, ho, depth+1)
	if err != nil {
		return nil, nil, fmt.Errorf("jump host %s: %w", hops[len(hops)-1], err)
	}
	return via, func(ctx context.Context) (net.Conn, error) {
		c, err := via.DialContext(ctx, "tcp", target)
		if err != nil {
			return nil, fmt.Errorf("connect %s via %s: %w", target, hops[len(hops)-1], err)
		}
		return c, nil
	}, nil
}

func splitJump(s string) []string {
	var out []string
	for h := range strings.SplitSeq(s, ",") {
		if h = strings.TrimSpace(h); h != "" && !strings.EqualFold(h, "none") {
			out = append(out, h)
		}
	}
	return out
}

// resolveHop turns "[ssh://][user@]host[:port]" into options via Resolve.
func (d *Dialer) resolveHop(ctx context.Context, hop string) (sshcfg.Options, error) {
	spec := strings.TrimPrefix(hop, "ssh://")
	var usr string
	if i := strings.LastIndex(spec, "@"); i >= 0 {
		usr, spec = spec[:i], spec[i+1:]
	}
	host, port := spec, 0
	if h, p, err := net.SplitHostPort(spec); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			host, port = h, n
		}
	}
	var o sshcfg.Options
	if d.Resolve != nil {
		var err error
		if o, err = d.Resolve(ctx, host); err != nil {
			return o, fmt.Errorf("resolve jump host %s: %w", host, err)
		}
	} else {
		o = sshcfg.Options{Alias: host, HostName: host}
	}
	if usr != "" {
		o.User = usr
	}
	if port != 0 {
		o.Port = port
	}
	return o, nil
}

// keepAlive probes the server every interval and closes the client after max
// consecutive unanswered probes, so dead links surface as errors instead of hangs.
func keepAlive(c *ssh.Client, interval time.Duration, maxMissed int) {
	if interval <= 0 {
		interval = defaultKeepAlive
	}
	if maxMissed <= 0 {
		maxMissed = defaultKeepAliveMax
	}
	done := make(chan struct{})
	go func() {
		_ = c.Wait()
		close(done)
	}()
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		missed := 0
		for {
			select {
			case <-done:
				return
			case <-tick.C:
			}
			reply := make(chan error, 1)
			go func() {
				_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
				reply <- err
			}()
			timeout := time.NewTimer(interval)
			select {
			case <-done:
				timeout.Stop()
				return
			case err := <-reply:
				timeout.Stop()
				if err != nil {
					missed = maxMissed
				} else {
					missed = 0
				}
			case <-timeout.C:
				missed++
			}
			if missed >= maxMissed {
				_ = c.Close()
				return
			}
		}
	}()
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}
