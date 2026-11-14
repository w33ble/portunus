package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/swipe"
	"github.com/w33ble/portunus/internal/transfer"
)

const (
	EventTransfers   = "transfers:update"
	EventConnections = "connections:changed"
	EventDrop        = "drop:files"
	// EventSwipe carries a swipe.Direction from the OS's own swipe gesture (macOS).
	EventSwipe = "nav:swipe"
)

// DropFiles reports files dropped from the OS onto a pane.
type DropFiles struct {
	Paths []string `json:"paths"`
	Pane  string   `json:"pane"` // data-pane attribute of the drop target, "" if none
	Dir   string   `json:"dir"`  // data-path of a folder dropped onto, "" for the pane's current directory
}

// Registration gives the frontend generated types for each event payload.
func init() {
	application.RegisterEvent[transfer.Update](EventTransfers)
	application.RegisterEvent[[]Connection](EventConnections)
	application.RegisterEvent[DropFiles](EventDrop)
	application.RegisterEvent[swipe.Direction](EventSwipe)
	application.RegisterEvent[prompt.HostKeyPrompt](prompt.EventHostKey)
	application.RegisterEvent[prompt.SecretPrompt](prompt.EventSecret)
	application.RegisterEvent[prompt.ConflictPrompt](prompt.EventConflict)
	application.RegisterEvent[string](prompt.EventResolved)
}
