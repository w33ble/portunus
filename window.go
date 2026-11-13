package main

import (
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/w33ble/portunus/internal/state"
)

const (
	defaultWidth  = 1280
	defaultHeight = 800
	minWidth      = 900
	minHeight     = 560

	// saveDelay debounces geometry saves while the window is being dragged or resized.
	saveDelay = 500 * time.Millisecond
	// screenWait bounds how long startup waits for the screen list, which Wails fills in asynchronously.
	screenWait = 2 * time.Second
)

var windowLimits = state.Limits{
	DefaultWidth: defaultWidth, DefaultHeight: defaultHeight,
	MinWidth: minWidth, MinHeight: minHeight,
}

// openMainWindow creates the main window where the user left it and keeps state.json up to date.
// It must run once the application has started: placement needs the connected screens.
func openMainWindow(app *application.App, store *state.Store, base application.WebviewWindowOptions) *application.WebviewWindow {
	canPosition := state.CanPositionWindows(runtime.GOOS, os.Getenv)

	var saved *state.Window
	if w, ok := store.Window(); ok {
		if !canPosition {
			w.Position = nil
		}
		saved = &w
	}

	screens := waitForScreens(app)
	placement := state.Place(saved, toStateScreens(screens), windowLimits)

	opts := base
	opts.Width, opts.Height = placement.Frame.Width, placement.Frame.Height
	applyPosition(&opts, placement, screens)
	if placement.Maximised {
		opts.StartState = application.WindowStateMaximised
	}

	win := app.Window.NewWithOptions(opts)

	tracker := state.NewTracker(store, win, state.TrackerOptions{
		Delay:    saveDelay,
		Position: canPosition,
		Logger:   slog.Default(),
	})
	for _, e := range []events.WindowEventType{
		events.Common.WindowDidMove,
		events.Common.WindowDidResize,
		events.Common.WindowMaximise,
		events.Common.WindowUnMaximise,
		events.Common.WindowFullscreen,
		events.Common.WindowUnFullscreen,
		events.Common.WindowRestore,
	} {
		win.OnWindowEvent(e, func(*application.WindowEvent) { tracker.Changed() })
	}
	if canPosition && !placement.Maximised && placement.Screen >= 0 {
		var once sync.Once
		want := placement.Frame
		win.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
			once.Do(func() {
				correctPosition(win, want.X, want.Y)
				tracker.Changed()
			})
		})
	}
	// A hook runs before the close handler destroys the window, so the frame can still be read.
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) { tracker.Flush() })
	// Quitting from the app menu may tear the window down without a closing event.
	app.OnShutdown(tracker.Flush)

	return win
}

// maxCorrection bounds the offset correctPosition will compensate, so a window the user already
// dragged elsewhere is left alone.
const maxCorrection = 100

// correctPosition re-places the window if it landed away from (wantX, wantY). Position reports the
// window's client area, but a reparenting X11 window manager treats a requested position as the
// frame's, so a restored window would otherwise creep by the frame size on every launch. The
// offset is measured rather than assumed because it depends on the window manager and theme.
func correctPosition(win *application.WebviewWindow, wantX, wantY int) {
	if win.IsMaximised() || win.IsFullscreen() || win.IsMinimised() {
		return
	}
	x, y := win.Position()
	dx, dy := wantX-x, wantY-y
	if (dx == 0 && dy == 0) || abs(dx) > maxCorrection || abs(dy) > maxCorrection {
		return
	}
	win.SetPosition(wantX+dx, wantY+dy)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func waitForScreens(app *application.App) []*application.Screen {
	deadline := time.Now().Add(screenWait)
	for {
		if screens := app.Screen.GetAll(); len(screens) > 0 {
			return screens
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func toStateScreens(screens []*application.Screen) []state.Screen {
	out := make([]state.Screen, len(screens))
	for i, s := range screens {
		out[i] = state.Screen{
			WorkArea: state.Rect{X: s.WorkArea.X, Y: s.WorkArea.Y, Width: s.WorkArea.Width, Height: s.WorkArea.Height},
			Primary:  s.IsPrimary,
		}
	}
	return out
}

// applyPosition hands the placement to Wails. GTK takes a position relative to a screen's work
// area; macOS takes absolute coordinates (its screen-relative variant rescales by the backing
// factor). A maximised macOS window is centered instead: moving it would undo the zoom.
func applyPosition(opts *application.WebviewWindowOptions, p state.Placement, screens []*application.Screen) {
	opts.InitialPosition = application.WindowCentered
	if p.Screen < 0 {
		return
	}
	if runtime.GOOS == "darwin" {
		if !p.Maximised {
			opts.InitialPosition = application.WindowXY
			opts.X, opts.Y = p.Frame.X, p.Frame.Y
		}
		return
	}
	screen := screens[p.Screen]
	opts.Screen = screen
	opts.InitialPosition = application.WindowXY
	opts.X, opts.Y = p.Frame.X-screen.WorkArea.X, p.Frame.Y-screen.WorkArea.Y
}
