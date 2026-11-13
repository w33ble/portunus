package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

// listIdleTimeout is how long an open listing may go unpulled before it's reaped.
const listIdleTimeout = 60 * time.Second

// ErrUnknownListing means the cursor was never issued or its listing already finished,
// was closed, or was reaped.
var ErrUnknownListing = errors.New("unknown or expired listing")

// listMsg is one hand-off from a listing's producer: a page, the end (last), or the failure.
type listMsg struct {
	entries []vfs.Entry
	last    bool
	err     error
}

// listSession is one in-progress directory listing. A producer goroutine runs fs.List and
// hands pages over an unbuffered channel, so it only reads ahead as fast as the caller
// pulls and a huge S3 prefix is never buffered whole.
type listSession struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	msgs   chan listMsg
	idle   time.Duration
	timer  *time.Timer
	next   sync.Mutex // serializes pulls on one cursor
}

// listings owns every open listing session.
type listings struct {
	idle time.Duration

	mu     sync.Mutex
	base   context.Context
	cancel context.CancelFunc
	open   map[string]*listSession
}

func newListings(idle time.Duration) *listings {
	base, cancel := context.WithCancel(context.Background())
	return &listings{idle: idle, base: base, cancel: cancel, open: map[string]*listSession{}}
}

// start begins listing dir on fs. The session outlives the call that started it, so it
// runs on the manager's context rather than the request's.
func (l *listings) start(fs vfs.FS, dir string) (*listSession, error) {
	id, err := newCursor()
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.base.Err() != nil {
		return nil, errors.New("listing service is shut down")
	}

	ctx, cancel := context.WithCancel(l.base)
	s := &listSession{id: id, ctx: ctx, cancel: cancel, msgs: make(chan listMsg), idle: l.idle}
	s.timer = time.AfterFunc(l.idle, func() { l.close(id) })
	l.open[id] = s
	go s.produce(fs, dir)
	return s, nil
}

func (l *listings) get(id string) (*listSession, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.open[id]
	if !ok {
		return nil, ErrUnknownListing
	}
	return s, nil
}

// close stops and forgets a listing. Unknown ids are ignored.
func (l *listings) close(id string) {
	l.mu.Lock()
	s := l.open[id]
	delete(l.open, id)
	l.mu.Unlock()

	if s != nil {
		s.timer.Stop()
		s.cancel()
	}
}

// closeAll stops every listing and refuses new ones.
func (l *listings) closeAll() {
	l.mu.Lock()
	l.cancel()
	open := l.open
	l.open = map[string]*listSession{}
	l.mu.Unlock()

	for _, s := range open {
		s.timer.Stop()
	}
}

// pull returns the listing's next page and closes the listing once it ends or fails.
// Callers that give up early (ctx canceled) abandon the listing too.
func (l *listings) pull(ctx context.Context, s *listSession) (ListPage, error) {
	s.next.Lock()
	defer s.next.Unlock()

	if !s.timer.Stop() {
		return ListPage{}, ErrUnknownListing
	}

	var msg listMsg
	select {
	case msg = <-s.msgs:
	case <-ctx.Done():
		l.close(s.id)
		return ListPage{}, ctx.Err()
	case <-s.ctx.Done():
		return ListPage{}, s.ctx.Err()
	}

	if msg.err != nil || msg.last {
		l.close(s.id)
		return ListPage{}, msg.err
	}
	s.timer.Reset(s.idle)
	return ListPage{Entries: msg.entries, Cursor: s.id}, nil
}

// produce runs fs.List, handing each page over as the caller pulls it, then the end marker
// or the error.
func (s *listSession) produce(fs vfs.FS, dir string) {
	err := fs.List(s.ctx, dir, func(page []vfs.Entry) error {
		return s.send(listMsg{entries: page})
	})
	if err != nil {
		_ = s.send(listMsg{err: err})
		return
	}
	_ = s.send(listMsg{last: true})
}

func (s *listSession) send(m listMsg) error {
	select {
	case s.msgs <- m:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func newCursor() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("new listing cursor: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
