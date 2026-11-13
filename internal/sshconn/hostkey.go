package sshconn

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/skeema/knownhosts"
	"golang.org/x/crypto/ssh"
	xknownhosts "golang.org/x/crypto/ssh/knownhosts"
)

// ErrHostKeyRejected means the user declined to trust an unknown host key.
var ErrHostKeyRejected = errors.New("sshconn: host key rejected")

// HostKeyMismatchError reports a presented host key that conflicts with known_hosts.
// It is never resolved by prompting.
type HostKeyMismatchError struct {
	Host        string // known_hosts name that matched
	KeyType     string // presented key type
	Fingerprint string // presented key, SHA256:...
	File        string // known_hosts file holding the conflicting entry
	Line        int
}

func (e *HostKeyMismatchError) Error() string {
	return fmt.Sprintf("host key for %s changed: server presented %s %s but %s:%d has a different key",
		e.Host, e.KeyType, e.Fingerprint, e.File, e.Line)
}

// HostKeyUnknownError is returned when StrictHostKeyChecking forbids trusting a new key,
// or when no Prompter is configured.
type HostKeyUnknownError struct {
	Host        string
	KeyType     string
	Fingerprint string
}

func (e *HostKeyUnknownError) Error() string {
	return fmt.Sprintf("host key for %s is not in known_hosts (%s %s)", e.Host, e.KeyType, e.Fingerprint)
}

// errNeedTrust aborts the handshake after the unknown key was captured (TOFU phase one).
var errNeedTrust = errors.New("sshconn: host key needs trust decision")

type pendingKey struct {
	name string // known_hosts name with port, as passed to the host key callback
	key  ssh.PublicKey
}

func nonEmpty(files []string) []string {
	var out []string
	for _, f := range files {
		if f != "" && f != "none" && f != os.DevNull {
			out = append(out, f)
		}
	}
	return out
}

// loadKnownHosts reads every existing known_hosts file. Missing files count as empty.
func (st *dialState) loadKnownHosts() (*knownhosts.HostKeyDB, error) {
	var files []string
	for _, f := range nonEmpty(append(append(append([]string(nil), st.o.UserKnownHostsFiles...), st.o.GlobalKnownHostsFiles...), st.d.AppKnownHosts)) {
		if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return knownhosts.NewDB()
	}
	return knownhosts.NewDB(files...)
}

// lookupName is what OpenSSH records and looks up: HostKeyAlias as-is (no port),
// otherwise host or [host]:port.
func (st *dialState) lookupAddr() string {
	if st.o.HostKeyAlias != "" {
		return net.JoinHostPort(st.o.HostKeyAlias, "22")
	}
	return net.JoinHostPort(st.host(), fmt.Sprint(st.port()))
}

func (st *dialState) hostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	err := st.db.HostKeyCallback()(hostname, remote, key)
	if err == nil {
		return nil
	}
	switch {
	case knownhosts.IsHostKeyChanged(err):
		var ke *xknownhosts.KeyError
		_ = errors.As(err, &ke)
		mm := &HostKeyMismatchError{
			Host:        knownhosts.Normalize(hostname),
			KeyType:     key.Type(),
			Fingerprint: ssh.FingerprintSHA256(key),
		}
		if len(ke.Want) > 0 {
			mm.File, mm.Line = ke.Want[0].Filename, ke.Want[0].Line
		}
		st.hostErr = mm
		return mm
	case knownhosts.IsHostUnknown(err):
		return st.unknownHostKey(hostname, key)
	default:
		st.hostErr = fmt.Errorf("verify host key for %s: %w", hostname, err)
		return st.hostErr
	}
}

func (st *dialState) unknownHostKey(hostname string, key ssh.PublicKey) error {
	unknown := &HostKeyUnknownError{Host: knownhosts.Normalize(hostname), KeyType: key.Type(), Fingerprint: ssh.FingerprintSHA256(key)}
	switch strings.ToLower(st.o.StrictHostKeyChecking) {
	case "yes", "true":
		st.hostErr = unknown
		return unknown
	case "no", "off", "false", "accept-new":
		if _, err := st.saveHostKey(hostname, key); err != nil {
			st.d.logger().Warn("could not record host key", "host", unknown.Host, "err", err)
		}
		return nil
	}
	if st.d.Prompter == nil {
		st.hostErr = unknown
		return unknown
	}
	st.pending = &pendingKey{name: hostname, key: key}
	return errNeedTrust
}

// canAppend reports whether path can receive an appended line without side effects.
func canAppend(path string) bool {
	if _, err := os.Stat(path); err == nil {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return false
		}
		_ = f.Close()
		return true
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".portunus-probe-*")
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return true
}

// knownHostsTarget picks the first writable user file, else the app file.
func (st *dialState) knownHostsTarget() (string, error) {
	for _, f := range nonEmpty(st.o.UserKnownHostsFiles) {
		if canAppend(f) {
			return f, nil
		}
	}
	if st.d.AppKnownHosts != "" {
		return st.d.AppKnownHosts, nil
	}
	return "", errors.New("no writable known_hosts file")
}

func (st *dialState) saveHostKey(name string, key ssh.PublicKey) (string, error) {
	if cert, ok := key.(*ssh.Certificate); ok {
		key = cert.Key
	}
	target, err := st.knownHostsTarget()
	if err != nil {
		return "", err
	}
	line := knownHostsLine(name, key, st.o.HashKnownHosts)
	if err := appendLine(target, line); err != nil {
		if st.d.AppKnownHosts == "" || target == st.d.AppKnownHosts {
			return "", err
		}
		st.d.logger().Warn("known_hosts not writable, using app file", "file", target, "err", err)
		target = st.d.AppKnownHosts
		if err := appendLine(target, line); err != nil {
			return "", err
		}
	}
	return target, nil
}

func knownHostsLine(name string, key ssh.PublicKey, hash bool) string {
	n := knownhosts.Normalize(name)
	if hash {
		n = xknownhosts.HashHostname(n)
	}
	return n + " " + key.Type() + " " + base64.StdEncoding.EncodeToString(key.Marshal()) + "\n"
}

// appendLine appends line to path, creating the file 0600 (and, for the app file, its dir 0700).
func appendLine(path, line string) error {
	if !canAppendDirExists(path) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err == nil && fi.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], fi.Size()-1); err == nil || errors.Is(err, io.EOF) {
			if last[0] != '\n' {
				line = "\n" + line
			}
		}
	}
	_, err = f.WriteString(line)
	return err
}

func canAppendDirExists(path string) bool {
	fi, err := os.Stat(filepath.Dir(path))
	return err == nil && fi.IsDir()
}
