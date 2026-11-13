package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/w33ble/portunus/internal/vfs"
)

// LocalID is the always-present local filesystem connection.
const LocalID = "local"

type TargetKind string

const (
	TargetLocal TargetKind = "local"
	TargetSSH   TargetKind = "ssh"
	TargetS3    TargetKind = "s3"
)

// Target says what to connect to. SSH uses Alias (from ~/.ssh/config) or Host/User/Port (quick connect).
type Target struct {
	Kind      TargetKind `json:"kind"`
	Alias     string     `json:"alias,omitempty"`
	User      string     `json:"user,omitempty"`
	Host      string     `json:"host,omitempty"`
	Port      int        `json:"port,omitempty"`
	ProfileID string     `json:"profileId,omitempty"`
}

// Connection is a live filesystem the panes and transfer engine can use.
type Connection struct {
	ID     string   `json:"id"`
	Kind   vfs.Kind `json:"kind"`
	Label  string   `json:"label"`
	Home   string   `json:"home"`
	Target Target   `json:"target"`
}

// ErrNotConnected is returned for IDs with no live connection and no known target.
var ErrNotConnected = errors.New("not connected")

// dialed is what a dialer returns. lost closes when the underlying transport dies.
type dialed struct {
	fs    vfs.FS
	label string
	lost  <-chan struct{}
}

type dialFunc func(ctx context.Context, t Target) (dialed, error)

type liveConn struct {
	conn Connection
	fs   vfs.FS
}

// registry owns live connections. Connects are single-flight per ID, and a dropped
// transport is forgotten so the next use redials the remembered target once.
type registry struct {
	dial     dialFunc
	onChange func([]Connection)

	group singleflight.Group

	mu    sync.Mutex
	live  map[string]*liveConn
	known map[string]Target
}

func newRegistry(local vfs.FS, dial dialFunc, onChange func([]Connection)) *registry {
	r := &registry{
		dial:     dial,
		onChange: onChange,
		live:     map[string]*liveConn{},
		known:    map[string]Target{},
	}

	home, err := local.Home(context.Background())
	if err != nil {
		home = "/"
	}
	r.live[LocalID] = &liveConn{
		conn: Connection{ID: LocalID, Kind: vfs.KindLocal, Label: "Local", Home: home, Target: Target{Kind: TargetLocal}},
		fs:   local,
	}
	return r
}

func targetID(t Target) (string, error) {
	switch t.Kind {
	case TargetLocal:
		return LocalID, nil
	case TargetSSH:
		if t.Alias != "" {
			return "ssh:" + t.Alias, nil
		}
		if t.Host == "" {
			return "", errors.New("ssh target needs an alias or host")
		}
		hostPort := t.Host
		if t.Port != 0 {
			hostPort = net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
		}
		if t.User != "" {
			return "ssh:" + t.User + "@" + hostPort, nil
		}
		return "ssh:" + hostPort, nil
	case TargetS3:
		if t.ProfileID == "" {
			return "", errors.New("s3 target needs a profile")
		}
		return "s3:" + t.ProfileID, nil
	default:
		return "", fmt.Errorf("unknown target kind %q", t.Kind)
	}
}

// Connect returns the live connection for t, dialing it if needed.
func (r *registry) Connect(ctx context.Context, t Target) (Connection, error) {
	id, err := targetID(t)
	if err != nil {
		return Connection{}, err
	}

	if lc := r.lookup(id); lc != nil {
		return lc.conn, nil
	}

	// Detach from the caller so one canceled pane doesn't fail a shared in-flight dial;
	// prompts still bound it, and the dialer applies its own timeouts.
	ch := r.group.DoChan(id, func() (any, error) {
		return r.connect(context.WithoutCancel(ctx), id, t)
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return Connection{}, res.Err
		}
		return res.Val.(Connection), nil
	case <-ctx.Done():
		return Connection{}, ctx.Err()
	}
}

func (r *registry) connect(ctx context.Context, id string, t Target) (Connection, error) {
	if lc := r.lookup(id); lc != nil {
		return lc.conn, nil
	}

	d, err := r.dial(ctx, t)
	if err != nil {
		return Connection{}, err
	}

	homeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	home, err := d.fs.Home(homeCtx)
	if err != nil {
		_ = d.fs.Close()
		return Connection{}, fmt.Errorf("find home directory: %w", err)
	}

	lc := &liveConn{
		conn: Connection{ID: id, Kind: d.fs.Kind(), Label: d.label, Home: home, Target: t},
		fs:   d.fs,
	}

	r.mu.Lock()
	r.live[id] = lc
	r.known[id] = t
	r.mu.Unlock()

	if d.lost != nil {
		go func() {
			<-d.lost
			r.forget(id, lc)
		}()
	}

	r.changed()
	return lc.conn, nil
}

// FS returns the filesystem for id, transparently redialing a dropped connection once.
func (r *registry) FS(ctx context.Context, id string) (vfs.FS, error) {
	if lc := r.lookup(id); lc != nil {
		return lc.fs, nil
	}

	r.mu.Lock()
	t, ok := r.known[id]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s: %w", id, ErrNotConnected)
	}

	if _, err := r.Connect(ctx, t); err != nil {
		return nil, fmt.Errorf("reconnect %s: %w", id, err)
	}
	if lc := r.lookup(id); lc != nil {
		return lc.fs, nil
	}
	return nil, fmt.Errorf("%s: %w", id, ErrNotConnected)
}

// Disconnect closes id and forgets its target so it won't be redialed.
func (r *registry) Disconnect(id string) error {
	if id == LocalID {
		return errors.New("the local connection cannot be closed")
	}

	r.mu.Lock()
	lc := r.live[id]
	delete(r.live, id)
	delete(r.known, id)
	r.mu.Unlock()

	if lc == nil {
		return nil
	}
	err := lc.fs.Close()
	r.changed()
	return err
}

func (r *registry) List() []Connection {
	r.mu.Lock()
	out := make([]Connection, 0, len(r.live))
	for _, lc := range r.live {
		out = append(out, lc.conn)
	}
	r.mu.Unlock()

	// Local first, then by label for a stable picker.
	slices.SortFunc(out, func(a, b Connection) int {
		if (a.ID == LocalID) != (b.ID == LocalID) {
			if a.ID == LocalID {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	return out
}

// CloseAll closes every connection, including local.
func (r *registry) CloseAll() {
	r.mu.Lock()
	live := r.live
	r.live = map[string]*liveConn{}
	r.known = map[string]Target{}
	r.mu.Unlock()

	for _, lc := range live {
		_ = lc.fs.Close()
	}
}

func (r *registry) lookup(id string) *liveConn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live[id]
}

// forget drops lc if it is still the live entry for id; the target stays known for redial.
func (r *registry) forget(id string, lc *liveConn) {
	r.mu.Lock()
	cur := r.live[id]
	if cur == lc {
		delete(r.live, id)
	}
	r.mu.Unlock()

	if cur == lc {
		_ = lc.fs.Close()
		r.changed()
	}
}

func (r *registry) changed() {
	if r.onChange != nil {
		r.onChange(r.List())
	}
}
