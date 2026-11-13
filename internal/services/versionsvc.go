package services

import "github.com/w33ble/portunus/internal/version"

// VersionService reports which build is running: a release version, or dev-<commit>.
type VersionService struct{}

func (s *VersionService) Info() version.Info {
	return version.Get()
}
