package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/w33ble/portunus/internal/connprefs"
	"github.com/w33ble/portunus/internal/profiles"
	"github.com/w33ble/portunus/internal/sshcfg"
)

// ConnectionService lists connectable targets and manages live connections.
type ConnectionService struct {
	app *App
}

// SSHHosts lists aliases from ~/.ssh/config (Include-aware, patterns skipped).
func (s *ConnectionService) SSHHosts() ([]sshcfg.Host, error) {
	return s.app.sshHosts()
}

// SSHHostDetails is what `ssh -G` resolves for an alias, shown read-only in the app.
type SSHHostDetails struct {
	HostName  string `json:"hostName"`
	User      string `json:"user"`
	Port      int    `json:"port"`
	ProxyJump string `json:"proxyJump"`
}

// SSHHostDetails resolves an alias the way ssh would connect to it.
func (s *ConnectionService) SSHHostDetails(ctx context.Context, alias string) (SSHHostDetails, error) {
	if s.app.sshErr != nil {
		return SSHHostDetails{}, s.app.sshErr
	}
	o, err := s.app.resolver.Resolve(ctx, alias)
	if err != nil {
		return SSHHostDetails{}, err
	}
	jump := o.ProxyJump
	if jump == "none" {
		jump = ""
	}
	return SSHHostDetails{HostName: o.HostName, User: o.User, Port: o.Port, ProxyJump: jump}, nil
}

// SSHAvailable reports why SSH can't be used, or "" when it can.
func (s *ConnectionService) SSHAvailable() string {
	if s.app.sshErr != nil {
		return s.app.sshErr.Error()
	}
	return ""
}

// Connect opens (or reuses) a connection. Concurrent calls for one target share a dial.
func (s *ConnectionService) Connect(ctx context.Context, t Target) (Connection, error) {
	return s.app.reg.Connect(ctx, t)
}

func (s *ConnectionService) Disconnect(id string) error {
	return s.app.reg.Disconnect(id)
}

// Connections lists live connections, local first.
func (s *ConnectionService) Connections() []Connection {
	return s.app.reg.List()
}

// SecretsSessionOnly reports that the OS keychain is unavailable, so saved S3 secrets
// last only until the app quits.
func (s *ConnectionService) SecretsSessionOnly() bool {
	so, ok := s.app.secrets.(interface{ SessionOnly() bool })
	return ok && so.SessionOnly()
}

func (s *ConnectionService) S3Profiles() []profiles.S3Profile {
	return s.app.profiles.List()
}

// SaveS3Profile creates or updates a profile. An empty secret keeps the stored one.
func (s *ConnectionService) SaveS3Profile(p profiles.S3Profile, secret string) (profiles.S3Profile, error) {
	saved, err := s.app.profiles.Save(p, secret)
	if err != nil {
		return profiles.S3Profile{}, err
	}

	// Close the live connection but keep the target: panes redial with the new settings.
	id, _ := targetID(Target{Kind: TargetS3, ProfileID: saved.ID})
	_ = s.app.reg.Reset(id)
	return saved, nil
}

func (s *ConnectionService) DeleteS3Profile(id string) error {
	connID, _ := targetID(Target{Kind: TargetS3, ProfileID: id})
	_ = s.app.reg.Disconnect(connID)
	if err := s.app.profiles.Delete(id); err != nil {
		return err
	}
	return s.app.forget(connID)
}

// Saved returns saved hosts, per-connection preferences and bookmarks. Entries for
// connections that no longer exist, like an alias removed from ~/.ssh/config, are dropped.
func (s *ConnectionService) Saved() (connprefs.Data, error) {
	if err := s.app.conns.Prune(s.app.knownConn()); err != nil {
		return connprefs.Data{}, err
	}
	return s.app.conns.Data(), nil
}

// SaveSSHHost creates or updates a saved host. Changing its address moves its
// preferences and bookmarks to the new connection ID.
func (s *ConnectionService) SaveSSHHost(h connprefs.Host) (connprefs.Host, error) {
	saved, err := s.app.conns.SaveHost(h)
	if err != nil {
		return connprefs.Host{}, err
	}
	s.app.refreshLimits()
	return saved, nil
}

// DeleteSSHHost forgets a saved host. A pane using it stays connected.
func (s *ConnectionService) DeleteSSHHost(id string) error {
	if err := s.app.conns.DeleteHost(id); err != nil {
		return err
	}
	s.app.refreshLimits()
	return nil
}

// SaveConnectionPrefs replaces one connection's preferences and bookmarks.
func (s *ConnectionService) SaveConnectionPrefs(connID string, p connprefs.Prefs, bookmarks []connprefs.Bookmark) error {
	if !s.app.knownConn()(connID) {
		return fmt.Errorf("connection %q isn't saved: %w", connID, ErrNotConnected)
	}
	if err := s.app.conns.SetConnection(connID, p, bookmarks); err != nil {
		return err
	}
	s.app.refreshLimits()
	return nil
}

// AddBookmark bookmarks a folder on a saved connection. A folder is bookmarked once.
func (s *ConnectionService) AddBookmark(b connprefs.Bookmark) (connprefs.Bookmark, error) {
	if !s.app.knownConn()(b.ConnID) {
		return connprefs.Bookmark{}, fmt.Errorf("save this connection before bookmarking its folders: %w", ErrNotConnected)
	}
	return s.app.conns.AddBookmark(b)
}

func (s *ConnectionService) RemoveBookmark(id string) error {
	return s.app.conns.RemoveBookmark(id)
}

func (a *App) sshHosts() ([]sshcfg.Host, error) {
	path := a.sshConfig
	if path == "" {
		var err error
		if path, err = sshcfg.DefaultConfigPath(); err != nil {
			return nil, err
		}
	}
	return sshcfg.ListHosts(path)
}

// knownConn reports whether a connection ID belongs to Local, a saved host, an S3 profile
// or an alias in ~/.ssh/config. When the config can't be read, every alias counts as known
// so a broken Include doesn't wipe their preferences.
func (a *App) knownConn() func(connID string) bool {
	known := map[string]bool{LocalID: true}
	for _, h := range a.conns.Data().Hosts {
		known[h.ConnID] = true
	}
	for _, p := range a.profiles.List() {
		known["s3:"+p.ID] = true
	}
	hosts, err := a.sshHosts()
	for _, h := range hosts {
		known["ssh:"+h.Alias] = true
	}
	if err != nil {
		return func(id string) bool { return known[id] || strings.HasPrefix(id, "ssh:") }
	}
	return func(id string) bool { return known[id] }
}

func (a *App) forget(connID string) error {
	if err := a.conns.Forget(connID); err != nil {
		return err
	}
	a.refreshLimits()
	return nil
}

func (a *App) refreshLimits() {
	a.engine.SetLimits(a.limits(a.settings.Get()))
}
