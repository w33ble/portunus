package sshcfg

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // %C token is defined as SHA-1 by OpenSSH
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LocateSSH finds the ssh binary: /usr/bin/ssh first (GUI apps on macOS get a minimal PATH), then PATH.
func LocateSSH() (string, error) {
	if fi, err := os.Stat("/usr/bin/ssh"); err == nil && !fi.IsDir() {
		return "/usr/bin/ssh", nil
	}
	p, err := exec.LookPath("ssh")
	if err != nil {
		return "", fmt.Errorf("sshcfg locate ssh: %w", err)
	}
	return p, nil
}

// Resolver turns an alias or quick-connect destination into Options via `ssh -G`.
type Resolver struct {
	SSHPath    string // from LocateSSH
	ConfigPath string // passed as -F when non-empty
	Home       string // for ~ expansion; default os.UserHomeDir
	LocalUser  string // for %u; default current user
}

// Resolve returns the effective options for a Host alias.
func (r Resolver) Resolve(ctx context.Context, alias string) (Options, error) {
	return r.run(ctx, alias, nil)
}

// ResolveQuick resolves host with an optional user and port override (zero values are unset).
func (r Resolver) ResolveQuick(ctx context.Context, user, host string, port int) (Options, error) {
	var args []string
	if user != "" {
		args = append(args, "-l", user)
	}
	if port > 0 {
		args = append(args, "-p", strconv.Itoa(port))
	}
	return r.run(ctx, host, args)
}

func (r Resolver) run(ctx context.Context, dest string, extra []string) (Options, error) {
	out, err := r.runRaw(ctx, dest, extra...)
	if err != nil {
		return Options{}, err
	}
	return r.parse(dest, out)
}

func (r Resolver) runRaw(ctx context.Context, dest string, extra ...string) (string, error) {
	if dest == "" || strings.HasPrefix(dest, "-") {
		return "", fmt.Errorf("sshcfg resolve: invalid destination %q", dest)
	}
	if r.SSHPath == "" {
		return "", errors.New("sshcfg resolve: no ssh binary")
	}
	args := []string{"-G"}
	if r.ConfigPath != "" {
		args = append(args, "-F", r.ConfigPath)
	}
	args = append(args, extra...)
	args = append(args, "--", dest)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.SSHPath, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("sshcfg resolve %s: %w", dest, ctx.Err())
		}
		return "", fmt.Errorf("sshcfg resolve %s: %w: %s", dest, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (r Resolver) parse(alias, out string) (Options, error) {
	home := r.Home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Options{}, fmt.Errorf("sshcfg home dir: %w", err)
		}
		home = h
	}
	localUser := r.LocalUser
	if localUser == "" {
		if u, err := user.Current(); err == nil {
			localUser = u.Username
		}
	}

	vals := map[string][]string{}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		k, v, _ := strings.Cut(line, " ")
		if k == "" {
			continue
		}
		k = strings.ToLower(k)
		vals[k] = append(vals[k], v)
	}
	o, err := parseScalars(alias, vals)
	if err != nil {
		return Options{}, err
	}

	ex := expander{home: home, user: localUser, host: o.HostName, port: o.Port, remote: o.User, alias: alias}
	ex.init()
	o.fillPaths(&ex, vals)
	return o, nil
}

func parseScalars(alias string, vals map[string][]string) (Options, error) {
	first := func(k string) string {
		if v := vals[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	o := Options{
		Alias:                 alias,
		HostName:              first("hostname"),
		User:                  first("user"),
		IdentitiesOnly:        first("identitiesonly") == "yes",
		StrictHostKeyChecking: first("stricthostkeychecking"),
		HashKnownHosts:        first("hashknownhosts") == "yes",
		ProxyJump:             first("proxyjump"),
		ProxyCommand:          first("proxycommand"),
	}
	if p := first("port"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Options{}, fmt.Errorf("sshcfg parse port %q: %w", p, err)
		}
		o.Port = n
	}
	var err error
	if o.ConnectTimeout, err = parseSeconds(first("connecttimeout")); err != nil {
		return Options{}, fmt.Errorf("sshcfg parse connecttimeout: %w", err)
	}
	if o.ServerAliveInterval, err = parseSeconds(first("serveraliveinterval")); err != nil {
		return Options{}, fmt.Errorf("sshcfg parse serveraliveinterval: %w", err)
	}
	if s := first("serveralivecountmax"); s != "" {
		if o.ServerAliveCountMax, err = strconv.Atoi(s); err != nil {
			return Options{}, fmt.Errorf("sshcfg parse serveralivecountmax: %w", err)
		}
	}

	return o, nil
}

func (o *Options) fillPaths(ex *expander, vals map[string][]string) {
	first := func(k string) string {
		if v := vals[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	for _, f := range vals["identityfile"] {
		if p := ex.path(f); identityExists(p) {
			o.IdentityFiles = append(o.IdentityFiles, p)
		}
	}
	for _, f := range vals["certificatefile"] {
		o.CertificateFiles = append(o.CertificateFiles, ex.path(f))
	}
	o.UserKnownHostsFiles = ex.pathList(first("userknownhostsfile"))
	o.GlobalKnownHostsFiles = ex.pathList(first("globalknownhostsfile"))
	if a := first("hostkeyalias"); a != "" && !strings.EqualFold(a, "none") {
		o.HostKeyAlias = ex.tokens(a)
	}
	o.IdentityAgent = ex.agent(first("identityagent"))
}

func parseSeconds(s string) (time.Duration, error) {
	if s == "" || strings.EqualFold(s, "none") {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, err)
	}
	return time.Duration(n) * time.Second, nil
}

func identityExists(p string) bool {
	for _, c := range []string{p, p + ".pub"} {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

type expander struct {
	home, user, host, remote, alias string
	port                            int
	hostname, shortHost, uid, hash  string
}

func (e *expander) init() {
	e.hostname, _ = os.Hostname()
	e.shortHost, _, _ = strings.Cut(e.hostname, ".")
	e.uid = strconv.Itoa(os.Getuid())
	sum := sha1.Sum([]byte(e.hostname + e.host + strconv.Itoa(e.port) + e.remote)) //nolint:gosec // see import
	e.hash = hex.EncodeToString(sum[:])
}

// tokens expands %h %p %r %u %n %d %C %l %L %i %%; unknown tokens are left as written.
func (e *expander) tokens(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		if v, ok := e.token(s[i]); ok {
			b.WriteString(v)
		} else {
			b.WriteByte('%')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (e *expander) token(c byte) (string, bool) {
	m := map[byte]string{
		'%': "%", 'h': e.host, 'p': strconv.Itoa(e.port), 'r': e.remote, 'u': e.user, 'n': e.alias,
		'd': e.home, 'C': e.hash, 'l': e.hostname, 'L': e.shortHost, 'i': e.uid,
	}
	v, ok := m[c]
	return v, ok
}

func (e *expander) tilde(s string) string {
	switch {
	case s == "~":
		return e.home
	case strings.HasPrefix(s, "~/"):
		return filepath.Join(e.home, s[2:])
	}
	return s
}

func (e *expander) path(s string) string { return e.tilde(e.tokens(s)) }

// pathList splits a space-joined path list from ssh -G; since paths aren't quoted there,
// a word only starts a new path when it looks like one (absolute, ~, or a drive letter).
func (e *expander) pathList(s string) []string {
	var out []string
	for w := range strings.FieldsSeq(s) {
		if n := len(out); n > 0 && !looksLikePath(w) {
			out[n-1] += " " + w
			continue
		}
		out = append(out, w)
	}
	for i := range out {
		out[i] = e.path(out[i])
	}
	return out
}

func looksLikePath(w string) bool {
	return strings.HasPrefix(w, "/") || strings.HasPrefix(w, "~") || strings.HasPrefix(w, "%") ||
		(len(w) > 2 && w[1] == ':' && (w[2] == '\\' || w[2] == '/'))
}

// agent maps IdentityAgent to "" (use $SSH_AUTH_SOCK), "none", or an expanded socket path.
func (e *expander) agent(v string) string {
	switch {
	case v == "" || v == "SSH_AUTH_SOCK" || v == "$SSH_AUTH_SOCK":
		return ""
	case strings.EqualFold(v, "none"):
		return "none"
	case strings.HasPrefix(v, "$"):
		if p := os.Getenv(v[1:]); p != "" {
			return e.path(p)
		}
		return "none"
	}
	return e.path(v)
}
