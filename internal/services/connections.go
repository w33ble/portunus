package services

import (
	"context"

	"github.com/w33ble/portunus/internal/profiles"
	"github.com/w33ble/portunus/internal/sshcfg"
)

// ConnectionService lists connectable targets and manages live connections.
type ConnectionService struct {
	app *App
}

// SSHHosts lists aliases from ~/.ssh/config (Include-aware, patterns skipped).
func (s *ConnectionService) SSHHosts() ([]sshcfg.Host, error) {
	path := s.app.sshConfig
	if path == "" {
		var err error
		if path, err = sshcfg.DefaultConfigPath(); err != nil {
			return nil, err
		}
	}
	return sshcfg.ListHosts(path)
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

	// Drop a live connection so the next use picks up the new settings.
	id, _ := targetID(Target{Kind: TargetS3, ProfileID: saved.ID})
	_ = s.app.reg.Disconnect(id)
	return saved, nil
}

func (s *ConnectionService) DeleteS3Profile(id string) error {
	connID, _ := targetID(Target{Kind: TargetS3, ProfileID: id})
	_ = s.app.reg.Disconnect(connID)
	return s.app.profiles.Delete(id)
}
