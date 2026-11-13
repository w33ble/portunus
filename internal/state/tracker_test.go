package state

import (
	"sync"
	"testing"
	"time"
)

type fakeWindow struct {
	mu                         sync.Mutex
	x, y, w, h                 int
	maximised, full, minimised bool
}

func (f *fakeWindow) set(fn func(*fakeWindow)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeWindow) Position() (int, int) { f.mu.Lock(); defer f.mu.Unlock(); return f.x, f.y }
func (f *fakeWindow) Size() (int, int)     { f.mu.Lock(); defer f.mu.Unlock(); return f.w, f.h }
func (f *fakeWindow) IsMaximised() bool    { f.mu.Lock(); defer f.mu.Unlock(); return f.maximised }
func (f *fakeWindow) IsFullscreen() bool   { f.mu.Lock(); defer f.mu.Unlock(); return f.full }
func (f *fakeWindow) IsMinimised() bool    { f.mu.Lock(); defer f.mu.Unlock(); return f.minimised }

func newTracker(t *testing.T, win *fakeWindow, position bool, delay time.Duration) (*Tracker, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return NewTracker(store, win, TrackerOptions{Delay: delay, Position: position}), dir
}

func savedWindow(t *testing.T, dir string) (Window, bool) {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s.Window()
}

func TestFlushWritesCurrentFrame(t *testing.T) {
	win := &fakeWindow{x: 120, y: 80, w: 1100, h: 700}
	tr, dir := newTracker(t, win, true, time.Hour)

	tr.Flush()

	want := Window{Width: 1100, Height: 700, Position: &Point{120, 80}}
	if got, ok := savedWindow(t, dir); !ok || !got.equal(want) {
		t.Fatalf("saved = %+v, %v; want %+v", got, ok, want)
	}
}

func TestMaximisedQuitKeepsTheNormalFrame(t *testing.T) {
	win := &fakeWindow{x: 120, y: 80, w: 1100, h: 700}
	tr, dir := newTracker(t, win, true, time.Hour)
	tr.Flush()

	win.set(func(f *fakeWindow) { f.x, f.y, f.w, f.h, f.maximised = 0, 0, 1920, 1080, true })
	tr.Flush()

	want := Window{Width: 1100, Height: 700, Position: &Point{120, 80}, Maximised: true}
	if got, _ := savedWindow(t, dir); !got.equal(want) {
		t.Fatalf("saved = %+v, want the pre-maximise frame %+v", got, want)
	}

	win.set(func(f *fakeWindow) { f.x, f.y, f.w, f.h, f.maximised = 300, 200, 1000, 650, false })
	tr.Flush()
	want = Window{Width: 1000, Height: 650, Position: &Point{300, 200}}
	if got, _ := savedWindow(t, dir); !got.equal(want) {
		t.Fatalf("after un-maximise saved = %+v, want %+v", got, want)
	}
}

func TestFullscreenAndMinimisedAreNeverRecorded(t *testing.T) {
	win := &fakeWindow{x: 120, y: 80, w: 1100, h: 700}
	tr, dir := newTracker(t, win, true, time.Hour)
	tr.Flush()

	win.set(func(f *fakeWindow) { f.x, f.y, f.w, f.h, f.full = 0, 0, 1920, 1080, true })
	tr.Flush()
	win.set(func(f *fakeWindow) { f.full, f.minimised, f.x, f.y = false, true, -32000, -32000 })
	tr.Flush()

	want := Window{Width: 1100, Height: 700, Position: &Point{120, 80}}
	if got, _ := savedWindow(t, dir); !got.equal(want) {
		t.Fatalf("saved = %+v, want %+v", got, want)
	}
}

func TestDestroyedWindowKeepsLastFrameAndMaximisedFlag(t *testing.T) {
	win := &fakeWindow{x: 120, y: 80, w: 1100, h: 700}
	tr, dir := newTracker(t, win, true, time.Hour)
	tr.Flush()
	win.set(func(f *fakeWindow) { f.maximised = true })
	tr.Flush()

	// Wails reports a destroyed window as 0x0 and not maximised.
	win.set(func(f *fakeWindow) { f.x, f.y, f.w, f.h, f.maximised = 0, 0, 0, 0, false })
	tr.Flush()

	want := Window{Width: 1100, Height: 700, Position: &Point{120, 80}, Maximised: true}
	if got, _ := savedWindow(t, dir); !got.equal(want) {
		t.Fatalf("saved = %+v, want %+v", got, want)
	}
}

func TestPositionlessTrackingKeepsEarlierPosition(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(dir)
	if err := store.SetWindow(Window{Width: 1000, Height: 700, Position: &Point{50, 60}}); err != nil {
		t.Fatal(err)
	}
	win := &fakeWindow{w: 1200, h: 800} // Wayland reports 0,0
	tr := NewTracker(store, win, TrackerOptions{Delay: time.Hour})

	tr.Flush()

	want := Window{Width: 1200, Height: 800, Position: &Point{50, 60}}
	if got, _ := savedWindow(t, dir); !got.equal(want) {
		t.Fatalf("saved = %+v, want %+v", got, want)
	}

	fresh := t.TempDir()
	store, _ = Open(fresh)
	NewTracker(store, win, TrackerOptions{Delay: time.Hour}).Flush()
	if got, ok := savedWindow(t, fresh); !ok || got.Position != nil || got.Width != 1200 {
		t.Fatalf("saved = %+v, %v; want size only", got, ok)
	}
}

func TestChangedDebouncesAndSavesTheLatestFrame(t *testing.T) {
	win := &fakeWindow{x: 10, y: 10, w: 1000, h: 700}
	tr, dir := newTracker(t, win, true, 300*time.Millisecond)

	for i := 1; i <= 5; i++ {
		win.set(func(f *fakeWindow) { f.x = 10 * i })
		tr.Changed()
	}
	if _, ok := savedWindow(t, dir); ok {
		t.Fatal("saved before the debounce delay elapsed")
	}

	want := Window{Width: 1000, Height: 700, Position: &Point{50, 10}}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got, ok := savedWindow(t, dir); ok {
			if !got.equal(want) {
				t.Fatalf("saved = %+v, want %+v", got, want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("debounced save never happened")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFlushCancelsPendingSave(t *testing.T) {
	win := &fakeWindow{x: 10, y: 10, w: 1000, h: 700}
	tr, dir := newTracker(t, win, true, 50*time.Millisecond)

	tr.Changed()
	tr.Flush()
	if _, ok := savedWindow(t, dir); !ok {
		t.Fatal("Flush did not write")
	}
	win.set(func(f *fakeWindow) { f.x = 99 })
	time.Sleep(150 * time.Millisecond)

	if got, _ := savedWindow(t, dir); got.Position.X != 10 {
		t.Fatalf("a save fired after Flush: %+v", got)
	}
}
