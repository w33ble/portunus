package services

import "github.com/w33ble/portunus/internal/prompt"

// PromptService answers questions the backend asked via prompt:* events.
type PromptService struct {
	app *App
}

// Pending returns unanswered prompts so a reloaded frontend can re-show them.
func (s *PromptService) Pending() prompt.Pending {
	return s.app.broker.Pending()
}

func (s *PromptService) RespondHostKey(id string, a prompt.HostKeyAnswer) error {
	return s.app.broker.RespondHostKey(id, a)
}

func (s *PromptService) RespondSecret(id string, a prompt.SecretAnswer) error {
	return s.app.broker.RespondSecret(id, a)
}

func (s *PromptService) RespondConflict(id string, a prompt.ConflictAnswer) error {
	return s.app.broker.RespondConflict(id, a)
}
