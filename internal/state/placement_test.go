package state

import "testing"

var limits = Limits{DefaultWidth: 1280, DefaultHeight: 800, MinWidth: 900, MinHeight: 560}

// A 1920x1080 primary with a 25px menu bar, like a Mac or a GTK panel.
var primary = Screen{WorkArea: Rect{0, 25, 1920, 1055}, Primary: true}

func saved(x, y, w, h int) *Window {
	return &Window{Width: w, Height: h, Position: &Point{x, y}}
}

func TestPlace(t *testing.T) {
	leftMonitor := Screen{WorkArea: Rect{-1600, 0, 1600, 900}}
	rightMonitor := Screen{WorkArea: Rect{1920, 0, 2560, 1440}}
	smallPrimary := Screen{WorkArea: Rect{0, 0, 1024, 700}, Primary: true}

	tests := []struct {
		name    string
		saved   *Window
		screens []Screen
		want    Placement
	}{
		{
			name:    "first launch centers the default size on the primary",
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{320, 25 + 127, 1280, 800}, Screen: 0},
		},
		{
			name:    "first launch centers on the primary even when it is not first",
			screens: []Screen{leftMonitor, primary},
			want:    Placement{Frame: Rect{320, 25 + 127, 1280, 800}, Screen: 1},
		},
		{
			name:    "no primary flag uses the first screen",
			screens: []Screen{{WorkArea: Rect{0, 0, 2000, 1000}}, leftMonitor},
			want:    Placement{Frame: Rect{360, 100, 1280, 800}, Screen: 0},
		},
		{
			name:    "default size is clamped on a small screen",
			screens: []Screen{smallPrimary},
			want:    Placement{Frame: Rect{0, 0, 1024, 700}, Screen: 0},
		},
		{
			name:    "saved frame on the primary is reused",
			saved:   saved(200, 100, 1000, 700),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{200, 100, 1000, 700}, Screen: 0},
		},
		{
			name:    "saved frame on a right-hand monitor is reused",
			saved:   saved(2000, 60, 1400, 900),
			screens: []Screen{primary, rightMonitor},
			want:    Placement{Frame: Rect{2000, 60, 1400, 900}, Screen: 1},
		},
		{
			name:    "negative coordinates on a left-hand monitor are reused",
			saved:   saved(-1500, 40, 1200, 800),
			screens: []Screen{leftMonitor, primary},
			want:    Placement{Frame: Rect{-1500, 40, 1200, 800}, Screen: 0},
		},
		{
			name:    "left monitor unplugged falls back to the primary at the saved size",
			saved:   saved(-1500, 40, 1200, 800),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{360, 25 + 127, 1200, 800}, Screen: 0},
		},
		{
			name:    "right monitor unplugged falls back to the primary",
			saved:   saved(2000, 60, 1400, 900),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{260, 25 + 77, 1400, 900}, Screen: 0},
		},
		{
			name:    "resolution change leaves the frame off screen",
			saved:   saved(1500, 600, 1000, 700),
			screens: []Screen{smallPrimary},
			want:    Placement{Frame: Rect{12, 0, 1000, 700}, Screen: 0},
		},
		{
			name:    "fallback clamps an oversized frame to the primary work area",
			saved:   saved(5000, 0, 3000, 2000),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{0, 25, 1920, 1055}, Screen: 0},
		},
		{
			name:    "a visible frame larger than its screen is capped to the work area",
			saved:   saved(100, 50, 2100, 1100),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{0, 25, 1920, 1055}, Screen: 0},
		},
		{
			name:    "frame half on a screen stays",
			saved:   saved(1420, 100, 1000, 700),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{1420, 100, 1000, 700}, Screen: 0},
		},
		{
			name:    "frame less than half visible is moved back",
			saved:   saved(1520, 100, 1000, 700),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{460, 25 + 177, 1000, 700}, Screen: 0},
		},
		{
			name:    "frame spanning two monitors counts as visible and picks the larger share",
			saved:   saved(1420, 100, 1000, 700),
			screens: []Screen{primary, rightMonitor},
			want:    Placement{Frame: Rect{1420, 100, 1000, 700}, Screen: 0},
		},
		{
			name:    "title bar above the work area is not draggable and is moved back",
			saved:   saved(300, -400, 1000, 700),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{460, 25 + 177, 1000, 700}, Screen: 0},
		},
		{
			name:    "undersized saved frame is raised to the minimum",
			saved:   saved(100, 100, 300, 200),
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{100, 100, 900, 560}, Screen: 0},
		},
		{
			name:    "saved frame with a nonsense size falls back to the default",
			saved:   &Window{Position: &Point{10, 10}},
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{320, 25 + 127, 1280, 800}, Screen: 0},
		},
		{
			name:    "no saved position (Wayland) centers at the saved size",
			saved:   &Window{Width: 1100, Height: 650},
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{410, 25 + 202, 1100, 650}, Screen: 0},
		},
		{
			name:    "maximised is carried through",
			saved:   &Window{Width: 1000, Height: 700, Position: &Point{200, 100}, Maximised: true},
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{200, 100, 1000, 700}, Screen: 0, Maximised: true},
		},
		{
			name:    "maximised survives the off-screen fallback",
			saved:   &Window{Width: 1000, Height: 700, Position: &Point{9000, 100}, Maximised: true},
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{460, 25 + 177, 1000, 700}, Screen: 0, Maximised: true},
		},
		{
			name:    "maximised without a usable frame still maximises at the default size",
			saved:   &Window{Maximised: true},
			screens: []Screen{primary},
			want:    Placement{Frame: Rect{320, 25 + 127, 1280, 800}, Screen: 0, Maximised: true},
		},
		{
			name:    "no screens keeps the saved size and reports no position",
			saved:   saved(10, 10, 1000, 700),
			screens: nil,
			want:    Placement{Frame: Rect{0, 0, 1000, 700}, Screen: -1},
		},
		{
			name: "no screens and no state uses the default size",
			want: Placement{Frame: Rect{0, 0, 1280, 800}, Screen: -1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Place(tc.saved, tc.screens, limits); got != tc.want {
				t.Errorf("Place = %+v, want %+v", got, tc.want)
			}
		})
	}
}
