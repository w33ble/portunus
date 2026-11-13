// Package prompt brokers questions the backend must ask the user mid-operation
// (host key trust, secrets, file conflicts). Each Ask call emits a typed event and
// blocks until the UI responds, the context ends, or the broker closes.
package prompt

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/w33ble/portunus/internal/vfs"
)

// Event names emitted to the UI. Payloads are HostKeyPrompt, SecretPrompt, ConflictPrompt.
const (
	EventHostKey  = "prompt:hostkey"
	EventSecret   = "prompt:secret"
	EventConflict = "prompt:conflict"
	// EventResolved carries the ID of a prompt that no longer needs an answer
	// (answered elsewhere, or its context ended) so the UI can dismiss it.
	EventResolved = "prompt:resolved"
)

// ErrClosed is returned by pending and new Ask calls after Close.
var ErrClosed = errors.New("prompt: broker closed")

// ErrUnknownPrompt is returned by Respond* for IDs that are not pending.
var ErrUnknownPrompt = errors.New("prompt: unknown or already answered prompt")

type HostKeyRequest struct {
	Host        string `json:"host"`        // alias or hostname the user picked
	Address     string `json:"address"`     // host:port actually dialed
	KeyType     string `json:"keyType"`     // e.g. ssh-ed25519
	Fingerprint string `json:"fingerprint"` // SHA256:...
	SaveTo      string `json:"saveTo"`      // known_hosts file the key will be written to
}

type HostKeyAnswer struct {
	Accept bool `json:"accept"`
}

type SecretRequest struct {
	Title   string `json:"title"`   // e.g. "Passphrase for ~/.ssh/id_ed25519"
	Message string `json:"message"` // server instruction or context; may be empty
	Label   string `json:"label"`   // field label, e.g. "Passphrase", "Password", or a keyboard-interactive question
	Echo    bool   `json:"echo"`    // show input in clear text
}

type SecretAnswer struct {
	Value  string `json:"value"`
	Cancel bool   `json:"cancel"`
}

type ConflictAction string

const (
	ConflictOverwrite ConflictAction = "overwrite"
	ConflictSkip      ConflictAction = "skip"
	ConflictCancel    ConflictAction = "cancel" // cancel the whole job
)

type ConflictRequest struct {
	JobID string    `json:"jobId"`
	Src   vfs.Entry `json:"src"`
	Dst   vfs.Entry `json:"dst"`
}

type ConflictAnswer struct {
	Action     ConflictAction `json:"action"`
	ApplyToAll bool           `json:"applyToAll"`
}

type HostKeyPrompt struct {
	ID      string         `json:"id"`
	Request HostKeyRequest `json:"request"`
}

type SecretPrompt struct {
	ID      string        `json:"id"`
	Request SecretRequest `json:"request"`
}

type ConflictPrompt struct {
	ID      string          `json:"id"`
	Request ConflictRequest `json:"request"`
}

// Pending lists every unanswered prompt, oldest first per kind.
type Pending struct {
	HostKeys  []HostKeyPrompt  `json:"hostKeys"`
	Secrets   []SecretPrompt   `json:"secrets"`
	Conflicts []ConflictPrompt `json:"conflicts"`
}

// Emitter delivers an event to the UI.
type Emitter func(name string, data any)

type kind int

const (
	kindHostKey kind = iota
	kindSecret
	kindConflict
)

type waiter struct {
	kind    kind
	seq     uint64
	payload any      // *Prompt struct as emitted
	answer  chan any // buffered(1); receives the typed answer
}

// Broker is safe for concurrent use.
type Broker struct {
	emit Emitter
	seq  atomic.Uint64

	mu      sync.Mutex
	closed  bool
	pending map[string]*waiter
	done    chan struct{}
}

func New(emit Emitter) *Broker {
	return &Broker{emit: emit, pending: map[string]*waiter{}, done: make(chan struct{})}
}

func (b *Broker) AskHostKey(ctx context.Context, req HostKeyRequest) (HostKeyAnswer, error) {
	return ask[HostKeyAnswer](ctx, b, kindHostKey, EventHostKey, func(id string) any {
		return HostKeyPrompt{ID: id, Request: req}
	})
}

func (b *Broker) AskSecret(ctx context.Context, req SecretRequest) (SecretAnswer, error) {
	return ask[SecretAnswer](ctx, b, kindSecret, EventSecret, func(id string) any {
		return SecretPrompt{ID: id, Request: req}
	})
}

func (b *Broker) AskConflict(ctx context.Context, req ConflictRequest) (ConflictAnswer, error) {
	return ask[ConflictAnswer](ctx, b, kindConflict, EventConflict, func(id string) any {
		return ConflictPrompt{ID: id, Request: req}
	})
}

func (b *Broker) RespondHostKey(id string, a HostKeyAnswer) error {
	return b.respond(id, kindHostKey, a)
}

func (b *Broker) RespondSecret(id string, a SecretAnswer) error {
	return b.respond(id, kindSecret, a)
}

func (b *Broker) RespondConflict(id string, a ConflictAnswer) error {
	switch a.Action {
	case ConflictOverwrite, ConflictSkip, ConflictCancel:
	default:
		return fmt.Errorf("prompt: invalid conflict action %q", a.Action)
	}
	return b.respond(id, kindConflict, a)
}

// Pending returns all unanswered prompts so a reloaded UI can re-render them.
func (b *Broker) Pending() Pending {
	b.mu.Lock()
	ws := make([]*waiter, 0, len(b.pending))
	for _, w := range b.pending {
		ws = append(ws, w)
	}
	b.mu.Unlock()

	slices.SortFunc(ws, func(a, b *waiter) int { return cmp.Compare(a.seq, b.seq) })

	p := Pending{HostKeys: []HostKeyPrompt{}, Secrets: []SecretPrompt{}, Conflicts: []ConflictPrompt{}}
	for _, w := range ws {
		switch v := w.payload.(type) {
		case HostKeyPrompt:
			p.HostKeys = append(p.HostKeys, v)
		case SecretPrompt:
			p.Secrets = append(p.Secrets, v)
		case ConflictPrompt:
			p.Conflicts = append(p.Conflicts, v)
		}
	}
	return p
}

// Close fails every pending and future Ask with ErrClosed.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.done)
}

func ask[A any](ctx context.Context, b *Broker, k kind, event string, build func(id string) any) (A, error) {
	var zero A

	seq := b.seq.Add(1)
	id := strconv.FormatUint(seq, 10)
	w := &waiter{kind: k, seq: seq, payload: build(id), answer: make(chan any, 1)}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return zero, ErrClosed
	}
	b.pending[id] = w
	b.mu.Unlock()

	b.emit(event, w.payload)

	select {
	case a := <-w.answer:
		return a.(A), nil
	case <-ctx.Done():
		b.drop(id)
		return zero, ctx.Err()
	case <-b.done:
		b.drop(id)
		return zero, ErrClosed
	}
}

func (b *Broker) respond(id string, k kind, answer any) error {
	b.mu.Lock()
	w, ok := b.pending[id]
	if !ok || w.kind != k {
		b.mu.Unlock()
		return ErrUnknownPrompt
	}
	delete(b.pending, id)
	b.mu.Unlock()

	w.answer <- answer
	b.emit(EventResolved, id)
	return nil
}

func (b *Broker) drop(id string) {
	b.mu.Lock()
	_, ok := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()

	if ok {
		b.emit(EventResolved, id)
	}
}
