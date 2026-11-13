package state

import (
	"log/slog"
	"sync"
	"time"
)

// Probe reads a live window's geometry. *application.WebviewWindow satisfies it.
type Probe interface {
	Position() (x, y int)
	Size() (width, height int)
	IsMaximised() bool
	IsFullscreen() bool
	IsMinimised() bool
}

// TrackerOptions configure a Tracker.
type TrackerOptions struct {
	// Delay debounces Changed.
	Delay time.Duration
	// Position records window positions; leave false where they cannot be read (Wayland).
	Position bool
	Logger   *slog.Logger
}

// Tracker keeps the window's normal frame in the Store. The frame is only updated while the
// window is neither maximised, fullscreen nor minimised, so a maximised or fullscreen quit
// still remembers where the window goes when it is restored.
type Tracker struct {
	store *Store
	probe Probe
	opts  TrackerOptions

	mu    sync.Mutex
	cur   Window
	saved Window
	timer *time.Timer
}

// NewTracker starts from the Store's saved window, if any.
func NewTracker(store *Store, probe Probe, opts TrackerOptions) *Tracker {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	t := &Tracker{store: store, probe: probe, opts: opts}
	t.cur, _ = store.Window()
	t.saved = t.cur
	return t
}

// Changed schedules a save once the window has stopped moving or resizing for the debounce delay.
func (t *Tracker) Changed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer == nil {
		t.timer = time.AfterFunc(t.opts.Delay, t.Flush)
		return
	}
	t.timer.Reset(t.opts.Delay)
}

// Flush samples the window and writes the state now if it changed. Safe to call after the
// window is gone: it then writes the last frame seen.
func (t *Tracker) Flush() {
	// Probe before taking the lock: probing runs on the UI thread, which must never wait on t.mu.
	obs, ok := t.observe()

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer != nil {
		t.timer.Stop()
	}
	if ok {
		t.cur = t.merge(obs)
	}
	if t.cur.equal(t.saved) {
		return
	}
	if err := t.store.SetWindow(t.cur); err != nil {
		t.opts.Logger.Warn("save window state", "err", err)
		return
	}
	t.saved = t.cur
}

type observation struct {
	width, height int
	position      Point
	maximised     bool
}

// observe returns false when the window is not in a state worth recording.
func (t *Tracker) observe() (observation, bool) {
	if t.probe.IsFullscreen() || t.probe.IsMinimised() {
		return observation{}, false
	}
	w, h := t.probe.Size()
	if w <= 0 || h <= 0 {
		// A destroyed window reports no size (and IsMaximised false), which must not overwrite the flag.
		return observation{}, false
	}
	o := observation{width: w, height: h, maximised: t.probe.IsMaximised()}
	if t.opts.Position {
		x, y := t.probe.Position()
		o.position = Point{x, y}
	}
	return o, true
}

func (t *Tracker) merge(o observation) Window {
	next := t.cur
	next.Maximised = o.maximised
	if o.maximised {
		return next
	}
	next.Width, next.Height = o.width, o.height
	if t.opts.Position {
		p := o.position
		next.Position = &p
	}
	return next
}
