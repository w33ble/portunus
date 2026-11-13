package services

import (
	"reflect"
	"testing"
)

func TestHandleDropForwardsPaneAndFolder(t *testing.T) {
	var got []DropFiles
	a := &App{emit: func(name string, data any) {
		if name == EventDrop {
			got = append(got, data.(DropFiles))
		}
	}}

	a.HandleDrop([]string{"/Users/me/a.txt"}, map[string]string{"data-pane": "left"})
	a.HandleDrop([]string{"/Users/me/b.txt"}, map[string]string{"data-pane": "right", "data-path": "/srv/www"})
	a.HandleDrop(nil, map[string]string{"data-pane": "left"})

	want := []DropFiles{
		{Paths: []string{"/Users/me/a.txt"}, Pane: "left"},
		{Paths: []string{"/Users/me/b.txt"}, Pane: "right", Dir: "/srv/www"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("emitted %+v, want %+v", got, want)
	}
}
