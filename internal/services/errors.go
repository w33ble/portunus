package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/vfs"
)

// ErrorKind lets the frontend branch on failures without parsing messages.
type ErrorKind string

const (
	ErrKindNotExist        ErrorKind = "not_exist"
	ErrKindExist           ErrorKind = "exist"
	ErrKindPermission      ErrorKind = "permission"
	ErrKindUnsupported     ErrorKind = "unsupported"
	ErrKindNotConnected    ErrorKind = "not_connected"
	ErrKindHostKeyMismatch ErrorKind = "hostkey_mismatch"
	ErrKindCanceled        ErrorKind = "canceled"
	ErrKindOther           ErrorKind = "other"
)

// ErrorCause is the `cause` payload of a rejected binding call.
type ErrorCause struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

// Classify maps an error to the kind the UI understands.
func Classify(err error) ErrorKind {
	var mismatch *sshconn.HostKeyMismatchError
	switch {
	case errors.As(err, &mismatch):
		return ErrKindHostKeyMismatch
	case errors.Is(err, context.Canceled), errors.Is(err, prompt.ErrClosed), errors.Is(err, prompt.ErrUnknownPrompt):
		return ErrKindCanceled
	case errors.Is(err, ErrNotConnected):
		return ErrKindNotConnected
	case errors.Is(err, vfs.ErrNotExist):
		return ErrKindNotExist
	case errors.Is(err, vfs.ErrExist):
		return ErrKindExist
	case errors.Is(err, vfs.ErrPermission):
		return ErrKindPermission
	case errors.Is(err, vfs.ErrUnsupported):
		return ErrKindUnsupported
	default:
		return ErrKindOther
	}
}

// MarshalError is the application-wide binding error marshaler.
func MarshalError(err error) []byte {
	b, jerr := json.Marshal(ErrorCause{Kind: Classify(err), Message: err.Error()})
	if jerr != nil {
		return nil
	}
	return b
}
