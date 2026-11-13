package sshdtest

import (
	"net"
	"os"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Question is one keyboard-interactive prompt and the answer the server expects.
type Question struct {
	Prompt string
	Echo   bool
	Answer string
}

// InProcOptions configures StartInProc. Auth methods left empty are disabled.
type InProcOptions struct {
	Password            string     // accepted password for User
	KeyboardInteractive []Question // all must be answered correctly
	AuthorizedKeys      []ssh.PublicKey
}

// InProc is an in-process SSH server for auth flows real sshd can't serve unprivileged.
// It authenticates and answers global requests (keepalive) but opens no channels.
type InProc struct {
	Endpoint
	mu       sync.Mutex
	attempts []string
}

// Attempts lists the auth methods the server saw, in order ("password", "keyboard-interactive", "publickey").
func (s *InProc) Attempts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.attempts...)
}

func (s *InProc) note(m string) {
	s.mu.Lock()
	s.attempts = append(s.attempts, m)
	s.mu.Unlock()
}

// StartInProc serves SSH on a random localhost port until test cleanup.
func StartInProc(t testing.TB, o InProcOptions) *InProc {
	t.Helper()
	const user = "tester"
	host := NewKey(t)
	s := &InProc{Host: "127.0.0.1", User: user, HostKey: host.Public}
	dir, err := os.MkdirTemp("", "ptinp")
	if err != nil {
		t.Fatalf("inproc tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	s.Dir = dir

	cfg := s.serverConfig(o, host)

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("inproc listen: %v", err)
	}
	s.Port = ln.Addr().(*net.TCPAddr).Port
	s.Addr = net.JoinHostPort(s.Host, strconv.Itoa(s.Port))

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		conns []net.Conn
	)
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
			wg.Go(func() { serveInProc(c, cfg) })
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
	return s
}

func (s *InProc) serverConfig(o InProcOptions, host Key) *ssh.ServerConfig {
	const user = "tester"
	cfg := &ssh.ServerConfig{}
	cfg.AddHostKey(host.Signer)
	if o.Password != "" {
		cfg.PasswordCallback = func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			s.note("password")
			if c.User() == user && string(pw) == o.Password {
				return nil, nil
			}
			return nil, errDenied
		}
	}
	if len(o.KeyboardInteractive) > 0 {
		cfg.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, ch ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			s.note("keyboard-interactive")
			qs := make([]string, len(o.KeyboardInteractive))
			echos := make([]bool, len(qs))
			for i, q := range o.KeyboardInteractive {
				qs[i], echos[i] = q.Prompt, q.Echo
			}
			answers, err := ch(c.User(), "test instruction", qs, echos)
			if err != nil || len(answers) != len(qs) {
				return nil, errDenied
			}
			for i, q := range o.KeyboardInteractive {
				if answers[i] != q.Answer {
					return nil, errDenied
				}
			}
			return nil, nil
		}
	}
	if len(o.AuthorizedKeys) > 0 {
		cfg.PublicKeyCallback = func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			s.note("publickey")
			for _, a := range o.AuthorizedKeys {
				if string(a.Marshal()) == string(k.Marshal()) {
					return nil, nil
				}
			}
			return nil, errDenied
		}
	}
	return cfg
}

type deniedError struct{}

func (deniedError) Error() string { return "denied" }

var errDenied error = deniedError{}

func serveInProc(c net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		_ = c.Close()
		return
	}
	defer func() { _ = sc.Close() }()
	go func() {
		for ch := range chans {
			_ = ch.Reject(ssh.Prohibited, "no channels")
		}
	}()
	for r := range reqs {
		if r.WantReply {
			_ = r.Reply(false, nil)
		}
	}
}
