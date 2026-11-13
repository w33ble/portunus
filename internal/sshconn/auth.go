package sshconn

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/w33ble/portunus/internal/prompt"
)

const (
	maxPromptTries = 3
	appleSSHAdd    = "/usr/bin/ssh-add"
)

// ErrBadPassphrase means the passphrase was wrong too many times.
var ErrBadPassphrase = errors.New("sshconn: incorrect passphrase")

func (st *dialState) authMethods() []ssh.AuthMethod {
	return []ssh.AuthMethod{
		ssh.PublicKeysCallback(st.publicKeys),
		ssh.RetryableAuthMethod(ssh.KeyboardInteractive(st.keyboardInteractive), maxPromptTries),
		ssh.RetryableAuthMethod(ssh.PasswordCallback(st.password), maxPromptTries),
	}
}

func (st *dialState) agentSock() string {
	switch st.o.IdentityAgent {
	case "none":
		return ""
	case "":
		return st.d.getenv("SSH_AUTH_SOCK")
	default:
		return st.o.IdentityAgent
	}
}

func (st *dialState) openAgent() {
	st.agent = nil
	sock := st.agentSock()
	if sock == "" {
		return
	}
	nd := net.Dialer{Timeout: 5 * time.Second}
	conn, err := nd.DialContext(st.ctx, "unix", sock)
	if err != nil {
		st.d.logger().Debug("ssh agent unavailable", "sock", sock, "err", err)
		return
	}
	st.agent = agent.NewClient(conn)
	st.agentConn = conn
}

func (st *dialState) closeAgent() {
	if st.agentConn != nil {
		_ = st.agentConn.Close()
	}
	st.agent, st.agentConn = nil, nil
}

// publicKeys implements the single publickey method. Order: agent keys matching a
// configured IdentityFile, configured files, then (unless IdentitiesOnly) every other
// agent key. Encrypted files are lazy: see lazySigner.
func (st *dialState) publicKeys() ([]ssh.Signer, error) {
	idents := st.identities()
	var agentSigners []ssh.Signer
	if st.agent != nil {
		var err error
		if agentSigners, err = st.agent.Signers(); err != nil {
			st.d.logger().Warn("list agent keys", "err", err)
		}
	}
	byKey := map[string]ssh.Signer{}
	for _, s := range agentSigners {
		byKey[string(s.PublicKey().Marshal())] = s
	}

	var out []ssh.Signer
	used := map[string]bool{}
	add := func(s ssh.Signer) {
		k := string(s.PublicKey().Marshal())
		if !used[k] {
			used[k] = true
			out = append(out, s)
		}
	}
	for _, id := range idents {
		if id.pub == nil {
			continue
		}
		if s, ok := byKey[string(id.pub.Marshal())]; ok {
			add(s)
		}
	}
	for _, id := range idents {
		if id.agentOnly {
			continue
		}
		s, err := st.fileSigner(id)
		if err != nil {
			st.d.logger().Warn("skipping identity file", "file", id.path, "err", err)
			continue
		}
		add(s)
	}
	if !st.o.IdentitiesOnly {
		for _, s := range agentSigners {
			add(s)
		}
	}
	return st.withCertificates(out), nil
}

// withCertificates puts a certificate signer ahead of each key a CertificateFile matches.
func (st *dialState) withCertificates(signers []ssh.Signer) []ssh.Signer {
	var certs []*ssh.Certificate
	for _, f := range st.o.CertificateFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		pk, _, _, _, err := ssh.ParseAuthorizedKey(b)
		if err != nil {
			continue
		}
		if c, ok := pk.(*ssh.Certificate); ok {
			certs = append(certs, c)
		}
	}
	if len(certs) == 0 {
		return signers
	}
	var out []ssh.Signer
	for _, s := range signers {
		for _, c := range certs {
			if bytes.Equal(c.Key.Marshal(), s.PublicKey().Marshal()) {
				if cs, err := ssh.NewCertSigner(c, s); err == nil {
					out = append(out, cs)
				}
			}
		}
		out = append(out, s)
	}
	return out
}

func (st *dialState) password() (string, error) {
	n := st.attempts["password"]
	st.attempts["password"]++
	return st.ask(prompt.SecretRequest{
		Title:   fmt.Sprintf("Password for %s@%s", st.user(), st.host()),
		Message: retryMessage(n),
		Label:   "Password",
	})
}

func (st *dialState) keyboardInteractive(name, instruction string, questions []string, echos []bool) ([]string, error) {
	if len(questions) == 0 {
		return nil, nil
	}
	n := st.attempts["kbd"]
	st.attempts["kbd"]++
	title := name
	if title == "" {
		title = fmt.Sprintf("Authentication for %s@%s", st.user(), st.host())
	}
	msg := instruction
	if m := retryMessage(n); m != "" {
		msg = strings.TrimSpace(m + "\n" + msg)
	}
	answers := make([]string, len(questions))
	for i, q := range questions {
		v, err := st.ask(prompt.SecretRequest{Title: title, Message: msg, Label: strings.TrimSpace(q), Echo: i < len(echos) && echos[i]})
		if err != nil {
			return nil, err
		}
		answers[i] = v
	}
	return answers, nil
}

func retryMessage(attempt int) string {
	if attempt == 0 {
		return ""
	}
	return "Permission denied, please try again."
}

// identity is one configured IdentityFile.
type identity struct {
	path      string
	pub       ssh.PublicKey // nil only for legacy-encrypted keys without a .pub
	agentOnly bool          // .pub selector or security-key stub: signing happens in the agent
	data      []byte
	encrypted bool

	mu     sync.Mutex
	signer ssh.Signer // set once decrypted (or immediately when unencrypted)
}

func (st *dialState) identities() []*identity {
	if st.identsReady {
		return st.idents
	}
	st.identsReady = true
	for _, p := range st.o.IdentityFiles {
		id, err := loadIdentity(p)
		if err != nil {
			st.d.logger().Warn("ignoring identity file", "file", p, "err", err)
			continue
		}
		st.idents = append(st.idents, id)
	}
	return st.idents
}

func readPub(path string) (ssh.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey(b)
	return pk, err
}

func isSecurityKeyFile(path string) bool {
	return strings.Contains(filepath.Base(strings.TrimSuffix(path, ".pub")), "_sk")
}

func loadIdentity(path string) (*identity, error) {
	if strings.HasSuffix(path, ".pub") {
		pub, err := readPub(path)
		if err != nil {
			return nil, err
		}
		return &identity{path: path, pub: pub, agentOnly: true}, nil
	}
	pubFromFile, pubErr := readPub(path + ".pub")
	if isSecurityKeyFile(path) {
		if pubErr != nil {
			return nil, fmt.Errorf("security key identity needs %s.pub: %w", path, pubErr)
		}
		return &identity{path: path, pub: pubFromFile, agentOnly: true}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && pubErr == nil {
			return &identity{path: path, pub: pubFromFile, agentOnly: true}, nil
		}
		return nil, err
	}
	id := &identity{path: path, data: data}
	signer, err := ssh.ParsePrivateKey(data)
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		id.signer, id.pub = signer, signer.PublicKey()
	case errors.As(err, &missing):
		id.encrypted = true
		id.pub = pubFromFile
		if id.pub == nil {
			id.pub = missing.PublicKey
		}
	default:
		return nil, err
	}
	if pubFromFile != nil {
		id.pub = pubFromFile
	}
	return id, nil
}

// fileSigner returns the signer to offer for a file identity: ready, lazy, or (legacy
// encrypted key without a known public key) decrypted right away, since the public key
// is unknowable otherwise.
func (st *dialState) fileSigner(id *identity) (ssh.Signer, error) {
	id.mu.Lock()
	s := id.signer
	id.mu.Unlock()
	if s != nil {
		return s, nil
	}
	if id.pub == nil {
		return st.decrypt(id)
	}
	return &lazySigner{st: st, id: id}, nil
}

// lazySigner offers an encrypted key's public half and only asks for the passphrase when
// the server accepted that key and Sign is called.
type lazySigner struct {
	st *dialState
	id *identity
}

func (l *lazySigner) PublicKey() ssh.PublicKey { return l.id.pub }

func (l *lazySigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	return l.SignWithAlgorithm(rand, data, "")
}

func (l *lazySigner) SignWithAlgorithm(rand io.Reader, data []byte, algo string) (*ssh.Signature, error) {
	s, err := l.st.decrypt(l.id)
	if err != nil {
		return nil, err
	}
	if as, ok := s.(ssh.AlgorithmSigner); ok {
		return as.SignWithAlgorithm(rand, data, algo)
	}
	if algo == "" || algo == s.PublicKey().Type() {
		return s.Sign(rand, data)
	}
	return nil, fmt.Errorf("key %s does not support algorithm %s", l.id.path, algo)
}

// decrypt prompts for the passphrase (once per Dial per key) and caches the result.
func (st *dialState) decrypt(id *identity) (ssh.Signer, error) {
	id.mu.Lock()
	defer id.mu.Unlock()
	if id.signer != nil {
		return id.signer, nil
	}
	if s := st.fromKeychain(id.pub); s != nil {
		id.signer = s
		return s, nil
	}
	var msg string
	for range maxPromptTries {
		pass, err := st.ask(prompt.SecretRequest{Title: "Passphrase for " + displayPath(id.path), Message: msg, Label: "Passphrase"})
		if err != nil {
			return nil, err
		}
		s, err := ssh.ParsePrivateKeyWithPassphrase(id.data, []byte(pass))
		if err == nil {
			id.signer = s
			if id.pub == nil {
				id.pub = s.PublicKey()
			}
			return s, nil
		}
		if !errors.Is(err, x509.IncorrectPasswordError) {
			return nil, fmt.Errorf("decrypt %s: %w", id.path, err)
		}
		msg = "Incorrect passphrase, try again."
	}
	return nil, st.setFatal(fmt.Errorf("%s: %w", id.path, ErrBadPassphrase))
}

// fromKeychain loads macOS keychain keys into the agent once, before the first passphrase
// prompt, and returns the agent's signer for pub if that made it available.
func (st *dialState) fromKeychain(pub ssh.PublicKey) ssh.Signer {
	if !st.d.LoadAppleKeychain || runtime.GOOS != "darwin" || st.agent == nil || pub == nil {
		return nil
	}
	st.keychain.Do(func() {
		cmd := exec.CommandContext(st.ctx, appleSSHAdd, "--apple-load-keychain")
		cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK="+st.agentSock())
		if err := cmd.Run(); err != nil {
			st.d.logger().Debug("ssh-add --apple-load-keychain failed", "err", err)
		}
	})
	signers, err := st.agent.Signers()
	if err != nil {
		return nil
	}
	for _, s := range signers {
		if bytes.Equal(s.PublicKey().Marshal(), pub.Marshal()) {
			return s
		}
	}
	return nil
}

func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return p
}
