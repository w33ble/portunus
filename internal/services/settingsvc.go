package services

import "github.com/w33ble/portunus/internal/settings"

// SettingsService reads and updates user preferences; changes apply immediately.
type SettingsService struct {
	app *App
}

func (s *SettingsService) Get() settings.Settings {
	return s.app.settings.Get()
}

// Save persists next and applies transfer limits and SSH options without a restart.
func (s *SettingsService) Save(next settings.Settings) (settings.Settings, error) {
	saved, err := s.app.settings.Save(next)
	if err != nil {
		return settings.Settings{}, err
	}
	s.app.applySettings(saved)
	return saved, nil
}
