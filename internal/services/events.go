package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/transfer"
	"github.com/w33ble/portunus/internal/vfs"
)

const (
	EventFilesPage   = "files:page"
	EventTransfers   = "transfers:update"
	EventConnections = "connections:changed"
	EventDrop        = "drop:files"
)

// FilePage is one batch of a streaming FileService.List call.
type FilePage struct {
	RequestID string      `json:"requestId"`
	Entries   []vfs.Entry `json:"entries"`
}

// DropFiles reports files dropped from the OS onto a pane.
type DropFiles struct {
	Paths []string `json:"paths"`
	Pane  string   `json:"pane"` // data-pane attribute of the drop target, "" if none
	Dir   string   `json:"dir"`  // data-path of a folder dropped onto, "" for the pane's current directory
}

// Registration gives the frontend generated types for each event payload.
func init() {
	application.RegisterEvent[FilePage](EventFilesPage)
	application.RegisterEvent[transfer.Update](EventTransfers)
	application.RegisterEvent[[]Connection](EventConnections)
	application.RegisterEvent[DropFiles](EventDrop)
	application.RegisterEvent[prompt.HostKeyPrompt](prompt.EventHostKey)
	application.RegisterEvent[prompt.SecretPrompt](prompt.EventSecret)
	application.RegisterEvent[prompt.ConflictPrompt](prompt.EventConflict)
	application.RegisterEvent[string](prompt.EventResolved)
}
