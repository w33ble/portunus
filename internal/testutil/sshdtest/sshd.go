// Package sshdtest runs real, unprivileged OpenSSH sshd instances (and an in-process
// SSH server for password flows) for integration tests.
//
// Real sshd cannot do password or keyboard-interactive auth as a non-root user (PAM and
// shadow lookups need privileges, and the test can't know the account password), so
// those flows run against StartInProc instead.
package sshdtest

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const sshdPath = "/usr/sbin/sshd"

// Endpoint identifies a started server.
type Endpoint struct {
	Host    string // 127.0.0.1
	Port    int
	Addr    string // host:port
	User    string
	HostKey ssh.PublicKey
	Dir     string // scratch dir owned by the server
}

// KnownHostsLine returns a known_hosts line trusting the server under name
// ("[name]:port" is used when name has no port and the port isn't 22).
func (e Endpoint) KnownHostsLine(name string) string {
	if !strings.Contains(name, ":") && e.Port != 22 {
		name = "[" + name + "]:" + strconv.Itoa(e.Port)
	}
	return name + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(e.HostKey))) + "\n"
}

// WriteSSHConfig writes an ssh_config with a Host block for alias pointing at the server
// and returns its path. extra lines are appended inside the block.
func (e Endpoint) WriteSSHConfig(t testing.TB, alias string, extra ...string) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s\n  HostName %s\n  Port %d\n  User %s\n", alias, e.Host, e.Port, e.User)
	for _, l := range extra {
		b.WriteString("  " + l + "\n")
	}
	p := filepath.Join(e.Dir, "ssh_config_"+alias)
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}
	return p
}

// Options configures Start.
type Options struct {
	// AuthorizedKeys are accepted for the current user; more can be added with Authorize.
	AuthorizedKeys []ssh.PublicKey
	// RSAHostKey makes the server also hold an RSA host key (Server.RSAHostKey).
	RSAHostKey bool
	// ExtraConfig is appended verbatim to sshd_config.
	ExtraConfig string
}

// Server is a running sshd.
type Server struct {
	Endpoint
	RSAHostKey ssh.PublicKey // nil unless Options.RSAHostKey
	authFile   string
	logPath    string
}

// Start launches sshd on 127.0.0.1 with a random port and temp host key and config.
// It skips the test when sshd is unavailable.
func Start(t testing.TB, o Options) *Server {
	t.Helper()
	if _, err := os.Stat(sshdPath); err != nil {
		t.Skipf("sshd not available: %v", err)
	}
	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}

	dir, err := os.MkdirTemp("", "ptsshd")
	if err != nil {
		t.Fatalf("sshd tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	host := NewKey(t)
	hostKeyPath := host.WriteFiles(t, dir, "host_ed25519", "", false)
	s := &Server{
		Host: "127.0.0.1", User: u.Username, HostKey: host.Public, Dir: dir,
		authFile: filepath.Join(dir, "authorized_keys"),
		logPath:  filepath.Join(dir, "sshd.log"),
	}
	s.Authorize(t, o.AuthorizedKeys...)
	hostKeys := "HostKey " + hostKeyPath
	if o.RSAHostKey {
		rk := NewRSAKey(t, 2048)
		s.RSAHostKey = rk.Public
		hostKeys += "\nHostKey " + rk.WriteFiles(t, dir, "host_rsa", "", false)
	}

	// The port is picked before sshd binds it, so retry when another process wins the race.
	var lastErr error
	for range 5 {
		port := freePort(t)
		cfgPath := filepath.Join(dir, "sshd_config")
		cfg := fmt.Sprintf(`ListenAddress 127.0.0.1
Port %d
%s
PidFile %s
AuthorizedKeysFile %s
AllowUsers %s
StrictModes no
UsePAM no
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
Subsystem sftp internal-sftp
LogLevel ERROR
%s
`, port, hostKeys, filepath.Join(dir, "sshd.pid"), s.authFile, u.Username, o.ExtraConfig)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
			t.Fatalf("write sshd_config: %v", err)
		}
		stop, err := run(cfgPath, s.logPath, port)
		if err != nil {
			lastErr = err
			continue
		}
		t.Cleanup(stop)
		s.Port = port
		s.Addr = net.JoinHostPort(s.Host, strconv.Itoa(port))
		return s
	}
	t.Fatalf("start sshd: %v", lastErr)
	return nil
}

// Authorize replaces the accepted client keys. sshd rereads the file on every login.
func (s *Server) Authorize(t testing.TB, keys ...ssh.PublicKey) {
	t.Helper()
	var b bytes.Buffer
	for _, k := range keys {
		b.Write(ssh.MarshalAuthorizedKey(k))
	}
	if err := os.WriteFile(s.authFile, b.Bytes(), 0o600); err != nil {
		t.Fatalf("write authorized_keys: %v", err)
	}
}

// Log returns sshd's stderr so far (for failure diagnostics).
func (s *Server) Log() string {
	b, _ := os.ReadFile(s.logPath)
	return string(b)
}

func freePort(t testing.TB) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// run starts sshd and waits until it accepts connections.
func run(cfgPath, logPath string, port int) (stop func(), err error) {
	logf, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, sshdPath, "-D", "-e", "-f", cfgPath)
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		cancel()
		_ = logf.Close()
		return nil, fmt.Errorf("start sshd: %w", err)
	}
	exited := make(chan struct{})
	var once sync.Once
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	stop = func() {
		once.Do(func() {
			cancel()
			<-exited
			_ = logf.Close()
		})
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			_ = logf.Close()
			b, _ := os.ReadFile(logPath)
			return nil, fmt.Errorf("sshd exited early: %s", strings.TrimSpace(string(b)))
		default:
		}
		d := net.Dialer{Timeout: time.Second}
		c, derr := d.DialContext(context.Background(), "tcp", addr)
		if derr == nil {
			_ = c.Close()
			return stop, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	return nil, fmt.Errorf("sshd did not listen on %s", addr)
}
