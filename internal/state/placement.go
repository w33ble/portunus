package state

// Rect is a rectangle in screen coordinates (DIP, origin at the primary screen's top-left).
type Rect struct{ X, Y, Width, Height int }

// Screen is one connected display.
type Screen struct {
	// WorkArea excludes menu bars, docks and panels.
	WorkArea Rect
	Primary  bool
}

// Limits are the window's size rules.
type Limits struct {
	DefaultWidth, DefaultHeight int
	MinWidth, MinHeight         int
}

// Placement is where to open the window.
type Placement struct {
	Frame Rect
	// Screen indexes the screens passed to Place; -1 when there were none and Frame has no position.
	Screen    int
	Maximised bool
}

const (
	// titleStrip is how much of the top edge must stay reachable for the window to be draggable.
	titleStrip = 32
	// A saved frame is kept when at least this share of it (and of its title strip) is visible.
	minVisibleShare = 0.5
)

// Place picks the frame for the main window from the saved state and the connected screens.
// A saved frame that is mostly on a screen is reused, with its size capped to that screen's work
// area. Otherwise, or with no saved position, the window is centered on the primary screen at the
// saved size (the default size on first launch), clamped to that work area.
func Place(saved *Window, screens []Screen, l Limits) Placement {
	size := Rect{Width: l.DefaultWidth, Height: l.DefaultHeight}
	var position *Point
	maximised := false
	if saved != nil {
		maximised = saved.Maximised
		if saved.Width > 0 && saved.Height > 0 {
			size = Rect{Width: saved.Width, Height: saved.Height}
			position = saved.Position
		}
	}

	if len(screens) == 0 {
		return Placement{Frame: size, Screen: -1, Maximised: maximised}
	}

	if position != nil {
		frame := Rect{position.X, position.Y, size.Width, size.Height}
		if owner := visibleOwner(frame, screens); owner >= 0 {
			frame = fit(frame, screens[owner].WorkArea, l)
			return Placement{Frame: frame, Screen: owner, Maximised: maximised}
		}
	}

	primary := primaryIndex(screens)
	wa := screens[primary].WorkArea
	w, h := clampSize(size.Width, size.Height, wa, l)
	return Placement{
		Frame:     Rect{wa.X + (wa.Width-w)/2, wa.Y + (wa.Height-h)/2, w, h},
		Screen:    primary,
		Maximised: maximised,
	}
}

// visibleOwner returns the screen showing most of frame, or -1 when too little of the frame
// (or of its title strip) lies on any screen.
func visibleOwner(frame Rect, screens []Screen) int {
	strip := Rect{frame.X, frame.Y, frame.Width, min(titleStrip, frame.Height)}

	owner, ownerArea, visible, stripVisible := -1, 0, 0, 0
	for i, s := range screens {
		area := overlap(frame, s.WorkArea)
		if area > ownerArea {
			owner, ownerArea = i, area
		}
		visible += area
		stripVisible += overlap(strip, s.WorkArea)
	}
	if owner < 0 || float64(min(visible, frame.Width*frame.Height)) < minVisibleShare*float64(frame.Width*frame.Height) {
		return -1
	}
	if float64(min(stripVisible, strip.Width*strip.Height)) < minVisibleShare*float64(strip.Width*strip.Height) {
		return -1
	}
	return owner
}

func primaryIndex(screens []Screen) int {
	for i, s := range screens {
		if s.Primary {
			return i
		}
	}
	return 0
}

// fit caps frame's size to the work area and, on an axis that shrank, shifts it back inside.
func fit(frame, wa Rect, l Limits) Rect {
	w, h := clampSize(frame.Width, frame.Height, wa, l)
	if w != frame.Width {
		frame.X = min(max(frame.X, wa.X), wa.X+wa.Width-w)
	}
	if h != frame.Height {
		frame.Y = min(max(frame.Y, wa.Y), wa.Y+wa.Height-h)
	}
	frame.Width, frame.Height = w, h
	return frame
}

// clampSize applies the minimum size, then caps to the work area (which wins when it is smaller).
func clampSize(w, h int, wa Rect, l Limits) (int, int) {
	return min(max(w, l.MinWidth), wa.Width), min(max(h, l.MinHeight), wa.Height)
}

func overlap(a, b Rect) int {
	w := min(a.X+a.Width, b.X+b.Width) - max(a.X, b.X)
	h := min(a.Y+a.Height, b.Y+b.Height) - max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}
