// Package connprefs stores what Portunus knows about connections beyond how to reach them:
// saved SSH hosts, per-connection preferences and bookmarks, all in connections.json.
//
// Preferences and bookmarks are keyed by connection ID (local, ssh:<alias>,
// ssh:<user@host:port>, s3:<profile>). Hosts from ~/.ssh/config are never stored here,
// only their preferences.
package connprefs

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
)

// MaxTransfers bounds a per-connection transfer limit, matching the global setting's cap.
const MaxTransfers = 32

// ErrNotFound is returned for a host or bookmark ID that isn't stored.
var ErrNotFound = errors.New("not found")

// Host is an SSH host saved in the app: a remembered quick connect.
type Host struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	User string `json:"user,omitzero"`
	Host string `json:"host"`
	Port int    `json:"port,omitzero"`
	// ConnID is the connection ID of this host's target, derived from User, Host and Port.
	ConnID string `json:"connId"`
}

// SSHConnID is the connection ID of a quick-connect SSH target.
func SSHConnID(user, host string, port int) string {
	hostPort := host
	if port != 0 {
		hostPort = net.JoinHostPort(host, strconv.Itoa(port))
	}
	if user != "" {
		return "ssh:" + user + "@" + hostPort
	}
	return "ssh:" + hostPort
}

// Color is a connection's color tag.
type Color string

const (
	ColorNone   Color = ""
	ColorRed    Color = "red"
	ColorOrange Color = "orange"
	ColorGreen  Color = "green"
	ColorBlue   Color = "blue"
	ColorPurple Color = "purple"
)

func (c Color) valid() bool {
	switch c {
	case ColorNone, ColorRed, ColorOrange, ColorGreen, ColorBlue, ColorPurple:
		return true
	default:
		return false
	}
}

// Prefs are per-connection settings. The zero value means "use the app defaults".
type Prefs struct {
	Color Color `json:"color,omitzero"`
	// OpenAt is the bookmarked folder a pane opens at on connect, instead of the connection's
	// home. It is always the path of one of the connection's bookmarks.
	OpenAt string `json:"openAt,omitzero"`
	// Transfers overrides the per-connection transfer limit; 0 uses the global setting.
	Transfers int `json:"transfers,omitzero"`
	// ShowHidden overrides the global hidden-files setting; nil follows it.
	ShowHidden *bool `json:"showHidden,omitzero"`
	// HideInPicker leaves the connection out of the connection picker.
	HideInPicker bool `json:"hideInPicker,omitzero"`
	// Favorite lists the connection first in the connection picker.
	Favorite bool `json:"favorite,omitzero"`
}

// Bookmark is a saved folder on one connection. A pinned bookmark is a favorite,
// listed from every pane whatever its connection.
type Bookmark struct {
	ID     string `json:"id"`
	ConnID string `json:"connId"`
	Path   string `json:"path"`
	// Name is shown instead of the folder name when set.
	Name   string `json:"name,omitzero"`
	Pinned bool   `json:"pinned,omitzero"`
}

// Data is everything the store holds.
type Data struct {
	Hosts     []Host           `json:"hosts"`
	Prefs     map[string]Prefs `json:"prefs"`
	Bookmarks []Bookmark       `json:"bookmarks"`
}

func (d Data) clone() Data {
	out := Data{
		Hosts:     slices.Clone(d.Hosts),
		Prefs:     maps.Clone(d.Prefs),
		Bookmarks: slices.Clone(d.Bookmarks),
	}
	if out.Hosts == nil {
		out.Hosts = []Host{}
	}
	if out.Prefs == nil {
		out.Prefs = map[string]Prefs{}
	}
	if out.Bookmarks == nil {
		out.Bookmarks = []Bookmark{}
	}
	for id, p := range out.Prefs {
		if p.ShowHidden != nil {
			v := *p.ShowHidden
			p.ShowHidden = &v
			out.Prefs[id] = p
		}
	}
	return out
}

func (d *Data) rekey(from, to string) {
	if from == to {
		return
	}
	if p, ok := d.Prefs[from]; ok {
		delete(d.Prefs, from)
		if _, taken := d.Prefs[to]; !taken {
			d.Prefs[to] = p
		}
	}
	for i := range d.Bookmarks {
		if d.Bookmarks[i].ConnID == from {
			d.Bookmarks[i].ConnID = to
		}
	}
}

func (d *Data) forget(connID string) {
	delete(d.Prefs, connID)
	d.Bookmarks = slices.DeleteFunc(d.Bookmarks, func(b Bookmark) bool { return b.ConnID == connID })
}

func normalizePrefs(p Prefs) (Prefs, error) {
	p.OpenAt = strings.TrimSpace(p.OpenAt)
	switch {
	case !p.Color.valid():
		return Prefs{}, fmt.Errorf("connprefs: unknown color %q", p.Color)
	case p.OpenAt != "" && !isAbs(p.OpenAt):
		return Prefs{}, fmt.Errorf("connprefs: open-at folder %q must be an absolute path", p.OpenAt)
	case p.Transfers < 0 || p.Transfers > MaxTransfers:
		return Prefs{}, fmt.Errorf("connprefs: transfer limit must be between 1 and %d", MaxTransfers)
	}
	return p, nil
}

func normalizeBookmark(b Bookmark) (Bookmark, error) {
	b.Path = strings.TrimSpace(b.Path)
	b.Name = strings.TrimSpace(b.Name)
	if !isAbs(b.Path) {
		return Bookmark{}, fmt.Errorf("connprefs: bookmark path %q must be an absolute path", b.Path)
	}
	return b, nil
}

// isAbs accepts vfs paths: '/'-separated and rooted at "/" or a drive ("C:/").
func isAbs(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && p[2] == '/' &&
		(p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z')
}
