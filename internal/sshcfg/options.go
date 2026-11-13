// Package sshcfg lists hosts from OpenSSH client config and resolves effective
// per-host options through `ssh -G`, the only resolver (so Match/Include/canonicalization
// behave exactly like the ssh CLI).
package sshcfg

import "time"

// Host is one connectable alias from a `Host` line (patterns excluded).
type Host struct {
	Alias  string `json:"alias"`
	Source string `json:"source"` // "path:line" of the Host line
}

// Options are the effective settings for one destination, with `~` and %-tokens expanded.
type Options struct {
	Alias    string // what the user picked (alias or quick-connect host)
	HostName string
	User     string
	Port     int

	// IdentityFiles lists configured identity files in order, filtered to ones that exist
	// (either the private key or its .pub, since agent-backed keys may only have a .pub).
	IdentityFiles  []string
	IdentitiesOnly bool
	// IdentityAgent is "" to use $SSH_AUTH_SOCK, "none" to disable the agent,
	// or an expanded socket path.
	IdentityAgent    string
	CertificateFiles []string

	HostKeyAlias          string
	UserKnownHostsFiles   []string
	GlobalKnownHostsFiles []string
	StrictHostKeyChecking string // "yes", "no", "ask", "accept-new", "off"
	HashKnownHosts        bool

	ProxyJump    string // raw value; "" or "none" means direct
	ProxyCommand string // non-empty means unsupported in MVP

	ConnectTimeout      time.Duration // 0 = default
	ServerAliveInterval time.Duration // 0 = default keepalive chosen by sshconn
	ServerAliveCountMax int
}

// KnownHostsKey is the name to look up in known_hosts: HostKeyAlias if set, else HostName.
func (o Options) KnownHostsKey() string {
	if o.HostKeyAlias != "" {
		return o.HostKeyAlias
	}
	return o.HostName
}
