package services

import (
	"os"
	"os/user"
	"path/filepath"

	"github.com/w33ble/portunus/internal/sshcfg"
)

// quickOptions builds connection options for a quick-connect target without the ssh
// binary: default identity files that exist, the user's known_hosts, and prompting for
// unknown host keys. homeDir is the user's home directory.
func quickOptions(t Target, homeDir, currentUser string) sshcfg.Options {
	username := t.User
	if username == "" {
		username = currentUser
	}
	port := t.Port
	if port == 0 {
		port = 22
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	var identities []string
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		p := filepath.Join(sshDir, name)
		if _, err := os.Stat(p); err == nil {
			identities = append(identities, p)
		}
	}

	return sshcfg.Options{
		Alias:                 t.Host,
		HostName:              t.Host,
		User:                  username,
		Port:                  port,
		IdentityFiles:         identities,
		UserKnownHostsFiles:   []string{filepath.Join(sshDir, "known_hosts")},
		StrictHostKeyChecking: "ask",
	}
}

func quickOptionsForCurrentUser(t Target) (sshcfg.Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return sshcfg.Options{}, err
	}
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return quickOptions(t, home, name), nil
}
