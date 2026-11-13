package sshconn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skeema/knownhosts"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/sshcfg"
)

// dialState carries everything one Dial needs across handshake attempts: decrypted keys
// stay cached so a TOFU or grace-timeout redial never re-prompts for a passphrase.
type dialState struct {
	d   *Dialer
	ctx context.Context
	o   sshcfg.Options

	mu    sync.Mutex
	fatal error

	// per-handshake
	prompted  bool
	timedOut  atomic.Bool
	stopTimer func() bool
	hostErr   error
	pending   *pendingKey
	db        *knownhosts.HostKeyDB
	agent     agent.ExtendedAgent
	agentConn net.Conn

	// per-Dial
	idents      []*identity
	identsReady bool
	keychain    sync.Once
	attempts    map[string]int
}

func newDialState(d *Dialer, ctx context.Context, o sshcfg.Options) *dialState {
	return &dialState{d: d, ctx: ctx, o: o, attempts: map[string]int{}}
}

func (st *dialState) host() string {
	if st.o.HostName != "" {
		return st.o.HostName
	}
	return st.o.Alias
}

func (st *dialState) port() int {
	if st.o.Port > 0 {
		return st.o.Port
	}
	return 22
}

func (st *dialState) user() string {
	if st.o.User != "" {
		return st.o.User
	}
	return currentUser()
}

func (st *dialState) target() string { return net.JoinHostPort(st.host(), strconv.Itoa(st.port())) }

func (st *dialState) fatalErr() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.fatal
}

func (st *dialState) setFatal(err error) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.fatal == nil {
		st.fatal = err
	}
	return st.fatal
}

// connect opens a stream, handshakes, and handles the two TOFU phases and the
// post-prompt redial. It does not start keepalives.
func (st *dialState) connect(open openFunc) (*ssh.Client, error) {
	trusted, redialed := false, false
	for {
		st.prompted, st.hostErr, st.pending = false, nil, nil

		conn, err := open(st.ctx)
		if err != nil {
			return nil, err
		}
		client, err := st.handshake(conn)
		if err == nil {
			return client, nil
		}

		switch {
		case st.pending != nil && !trusted:
			if err := st.trust(); err != nil {
				return nil, err
			}
			trusted = true
			continue
		case st.pending != nil:
			return nil, &HostKeyUnknownError{Host: st.target(), KeyType: st.pending.key.Type(), Fingerprint: ssh.FingerprintSHA256(st.pending.key)}
		case st.hostErr != nil:
			return nil, st.hostErr
		}
		if fatal := st.fatalErr(); fatal != nil {
			return nil, fmt.Errorf("ssh %s: %w", st.target(), fatal)
		}
		if cerr := st.ctx.Err(); cerr != nil {
			return nil, fmt.Errorf("ssh %s: %w", st.target(), cerr)
		}
		// The server may drop us at LoginGraceTime while the user typed a secret.
		if st.prompted && !redialed && !strings.Contains(err.Error(), "unable to authenticate") {
			redialed = true
			st.d.logger().Info("connection lost after prompt, redialing", "host", st.target())
			continue
		}
		return nil, fmt.Errorf("ssh %s: %w", st.target(), err)
	}
}

func (st *dialState) handshake(conn net.Conn) (*ssh.Client, error) {
	db, err := st.loadKnownHosts()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	st.db = db
	st.openAgent()
	defer st.closeAgent()

	lookup := st.lookupAddr()
	cfg := &ssh.ClientConfig{
		User:            st.user(),
		Auth:            st.authMethods(),
		HostKeyCallback: st.hostKeyCallback,
		ClientVersion:   "SSH-2.0-Portunus",
	}
	if algos := db.HostKeyAlgorithms(lookup); len(algos) > 0 {
		cfg.HostKeyAlgorithms = algos
	}

	timeout := st.o.ConnectTimeout
	if timeout <= 0 {
		timeout = defaultConnTimeout
	}
	st.timedOut.Store(false)
	timer := time.AfterFunc(timeout, func() {
		st.timedOut.Store(true)
		_ = conn.Close()
	})
	st.stopTimer = timer.Stop
	stop := context.AfterFunc(st.ctx, func() { _ = conn.Close() })
	sc, chans, reqs, err := ssh.NewClientConn(conn, lookup, cfg)
	stop()
	timer.Stop()
	if err != nil {
		_ = conn.Close()
		if st.timedOut.Load() {
			return nil, fmt.Errorf("handshake timed out after %s: %w", timeout, os.ErrDeadlineExceeded)
		}
		return nil, err
	}
	if err := st.ctx.Err(); err != nil {
		_ = sc.Close()
		return nil, err
	}
	return ssh.NewClient(sc, chans, reqs), nil
}

// trust is TOFU phase two: the connection is closed, ask, record, and let connect redial.
func (st *dialState) trust() error {
	key, name := st.pending.key, st.pending.name
	unknown := &HostKeyUnknownError{Host: knownhosts.Normalize(name), KeyType: key.Type(), Fingerprint: ssh.FingerprintSHA256(key)}
	if st.d.Prompter == nil {
		return unknown
	}
	saveTo, err := st.knownHostsTarget()
	if err != nil {
		return fmt.Errorf("trust host key for %s: %w", st.target(), err)
	}
	alias := st.o.Alias
	if alias == "" {
		alias = st.host()
	}
	ans, err := st.d.Prompter.AskHostKey(st.ctx, prompt.HostKeyRequest{
		Host:        alias,
		Address:     st.target(),
		KeyType:     key.Type(),
		Fingerprint: ssh.FingerprintSHA256(key),
		SaveTo:      saveTo,
	})
	if err != nil {
		return fmt.Errorf("ask host key for %s: %w", st.target(), err)
	}
	if !ans.Accept {
		return fmt.Errorf("%s: %w", st.target(), ErrHostKeyRejected)
	}
	if _, err := st.saveHostKey(name, key); err != nil {
		return fmt.Errorf("save host key for %s: %w", st.target(), err)
	}
	return nil
}

var errNoPrompter = errors.New("sshconn: no prompter configured")

// ask puts one secret question to the user. Cancellation or a broken broker is fatal
// for the whole Dial so later methods don't prompt again.
func (st *dialState) ask(req prompt.SecretRequest) (string, error) {
	if err := st.fatalErr(); err != nil {
		return "", err
	}
	if st.d.Prompter == nil {
		return "", errNoPrompter
	}
	st.prompted = true
	if st.stopTimer != nil {
		st.stopTimer() // the user's think time is not the server's fault
	}
	ans, err := st.d.Prompter.AskSecret(st.ctx, req)
	if err != nil {
		return "", st.setFatal(err)
	}
	if ans.Cancel {
		return "", st.setFatal(ErrCanceled)
	}
	return ans.Value, nil
}

// ErrCanceled means the user dismissed a secret prompt.
var ErrCanceled = errors.New("sshconn: canceled by user")
