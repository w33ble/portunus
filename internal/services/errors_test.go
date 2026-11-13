package services

import (
	"fmt"
	"testing"

	"github.com/w33ble/portunus/internal/prompt"
)

func TestClassifyUnknownPromptIsCanceled(t *testing.T) {
	err := fmt.Errorf("answer: %w", prompt.ErrUnknownPrompt)
	if got := Classify(err); got != ErrKindCanceled {
		t.Fatalf("Classify = %q, want %q", got, ErrKindCanceled)
	}
}
