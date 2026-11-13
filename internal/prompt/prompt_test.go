package prompt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu     sync.Mutex
	events []event
	notify chan event
}

type event struct {
	name string
	data any
}

func newRecorder() *recorder { return &recorder{notify: make(chan event, 16)} }

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, event{name, data})
	r.mu.Unlock()
	r.notify <- event{name, data}
}

func (r *recorder) next(t *testing.T) event {
	t.Helper()
	select {
	case e := <-r.notify:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return event{}
	}
}

func TestAskSecretRoundTrip(t *testing.T) {
	rec := newRecorder()
	b := New(rec.emit)

	type result struct {
		a   SecretAnswer
		err error
	}
	done := make(chan result, 1)
	go func() {
		a, err := b.AskSecret(context.Background(), SecretRequest{Title: "Passphrase", Label: "Passphrase"})
		done <- result{a, err}
	}()

	e := rec.next(t)
	if e.name != EventSecret {
		t.Fatalf("event = %q, want %q", e.name, EventSecret)
	}
	p := e.data.(SecretPrompt)
	if p.Request.Title != "Passphrase" {
		t.Fatalf("request title = %q", p.Request.Title)
	}

	if err := b.RespondSecret(p.ID, SecretAnswer{Value: "hunter2"}); err != nil {
		t.Fatal(err)
	}

	r := <-done
	if r.err != nil || r.a.Value != "hunter2" {
		t.Fatalf("got %+v, %v", r.a, r.err)
	}
	if e := rec.next(t); e.name != EventResolved || e.data != p.ID {
		t.Fatalf("want resolved %s, got %+v", p.ID, e)
	}
}

func TestRespondRejectsWrongKindAndUnknownID(t *testing.T) {
	rec := newRecorder()
	b := New(rec.emit)

	go func() { _, _ = b.AskHostKey(context.Background(), HostKeyRequest{Host: "h"}) }()
	p := rec.next(t).data.(HostKeyPrompt)

	if err := b.RespondSecret(p.ID, SecretAnswer{}); !errors.Is(err, ErrUnknownPrompt) {
		t.Fatalf("wrong-kind respond err = %v, want ErrUnknownPrompt", err)
	}
	if err := b.RespondHostKey("nope", HostKeyAnswer{}); !errors.Is(err, ErrUnknownPrompt) {
		t.Fatalf("unknown id err = %v, want ErrUnknownPrompt", err)
	}
	if err := b.RespondHostKey(p.ID, HostKeyAnswer{Accept: true}); err != nil {
		t.Fatal(err)
	}
	if err := b.RespondHostKey(p.ID, HostKeyAnswer{Accept: true}); !errors.Is(err, ErrUnknownPrompt) {
		t.Fatalf("double respond err = %v, want ErrUnknownPrompt", err)
	}
}

func TestRespondConflictValidatesAction(t *testing.T) {
	rec := newRecorder()
	b := New(rec.emit)

	go func() { _, _ = b.AskConflict(context.Background(), ConflictRequest{JobID: "j"}) }()
	p := rec.next(t).data.(ConflictPrompt)

	if err := b.RespondConflict(p.ID, ConflictAnswer{Action: "explode"}); err == nil {
		t.Fatal("invalid action accepted")
	}
	if len(b.Pending().Conflicts) != 1 {
		t.Fatal("invalid answer must leave the prompt pending")
	}
}

func TestContextCancelUnblocksAndResolves(t *testing.T) {
	rec := newRecorder()
	b := New(rec.emit)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := b.AskSecret(ctx, SecretRequest{})
		done <- err
	}()
	p := rec.next(t).data.(SecretPrompt)

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if e := rec.next(t); e.name != EventResolved || e.data != p.ID {
		t.Fatalf("want resolved %s, got %+v", p.ID, e)
	}
	if n := len(b.Pending().Secrets); n != 0 {
		t.Fatalf("pending secrets = %d after cancel", n)
	}
}

func TestPendingListsOutstandingInOrder(t *testing.T) {
	rec := newRecorder()
	b := New(rec.emit)

	for _, host := range []string{"a", "b", "c"} {
		go func() { _, _ = b.AskHostKey(context.Background(), HostKeyRequest{Host: host}) }()
		rec.next(t)
	}
	go func() { _, _ = b.AskSecret(context.Background(), SecretRequest{Title: "s"}) }()
	rec.next(t)

	p := b.Pending()
	if len(p.HostKeys) != 3 || len(p.Secrets) != 1 || len(p.Conflicts) != 0 {
		t.Fatalf("pending = %+v", p)
	}
	// Each Ask was observed before the next started, so seq order is a, b, c.
	for i, want := range []string{"a", "b", "c"} {
		if got := p.HostKeys[i].Request.Host; got != want {
			t.Fatalf("host key %d = %q, want %q", i, got, want)
		}
	}
}

func TestCloseFailsPendingAndFutureAsks(t *testing.T) {
	rec := newRecorder()
	b := New(rec.emit)

	done := make(chan error, 1)
	go func() {
		_, err := b.AskHostKey(context.Background(), HostKeyRequest{})
		done <- err
	}()
	rec.next(t)

	b.Close()
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Fatalf("pending err = %v, want ErrClosed", err)
	}
	if _, err := b.AskSecret(context.Background(), SecretRequest{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("future err = %v, want ErrClosed", err)
	}
	b.Close() // idempotent
}
