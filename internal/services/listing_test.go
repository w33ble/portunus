package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/profiles"
	"github.com/w33ble/portunus/internal/vfs"
)

const listWait = 5 * time.Second

// pagedFS lists scripted pages per directory. It records how far the producer ran and
// signals when List returns, so tests can see goroutines stop.
type pagedFS struct {
	vfs.FS
	pages map[string][][]vfs.Entry
	err   error         // returned by List after the last page
	gate  chan struct{} // when non-nil, List waits for it before each page after the first
	start chan struct{} // when non-nil, List waits for it before doing anything

	produced atomic.Int32  // page callbacks invoked
	returned chan struct{} // closed when the first List call returns
	once     sync.Once
}

func newPagedFS(pages ...[]vfs.Entry) *pagedFS {
	return &pagedFS{pages: map[string][][]vfs.Entry{"/d": pages}, returned: make(chan struct{})}
}

func (f *pagedFS) Kind() vfs.Kind                       { return vfs.KindLocal }
func (f *pagedFS) Home(context.Context) (string, error) { return "/", nil }

func (f *pagedFS) List(ctx context.Context, dir string, page func([]vfs.Entry) error) error {
	defer f.once.Do(func() { close(f.returned) })
	if f.start != nil {
		select {
		case <-f.start:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for i, p := range f.pages[dir] {
		if i > 0 && f.gate != nil {
			select {
			case <-f.gate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		f.produced.Add(1)
		if err := page(p); err != nil {
			return err
		}
	}
	return f.err
}

func (f *pagedFS) stopped(t *testing.T) {
	t.Helper()
	select {
	case <-f.returned:
	case <-time.After(listWait):
		t.Fatal("producer goroutine still running")
	}
}

func ents(names ...string) []vfs.Entry {
	es := make([]vfs.Entry, len(names))
	for i, n := range names {
		es[i] = vfs.Entry{Name: n, Path: "/d/" + n}
	}
	return es
}

func entryNames(es []vfs.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

func newListService(fs vfs.FS, idle time.Duration) *FileService {
	reg := newRegistry(fs, nil, func([]Connection) {})
	return &FileService{app: &App{reg: reg, lists: newListings(idle)}}
}

func (s *FileService) openListings() int {
	s.app.lists.mu.Lock()
	defer s.app.lists.mu.Unlock()
	return len(s.app.lists.open)
}

func wantPage(t *testing.T, p ListPage, wantMore bool, names ...string) {
	t.Helper()
	got := entryNames(p.Entries)
	if fmt.Sprint(got) != fmt.Sprint(names) {
		t.Fatalf("entries = %v, want %v", got, names)
	}
	if (p.Cursor != "") != wantMore {
		t.Fatalf("cursor = %q, want more = %v", p.Cursor, wantMore)
	}
}

func TestListReturnsPagesInOrderAndEndsWithAnEmptyCursor(t *testing.T) {
	fs := newPagedFS(ents("a", "b"), ents("c"), ents("d", "e"))
	s := newListService(fs, time.Minute)
	ctx := context.Background()

	p, err := s.List(ctx, LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, p, true, "a", "b")

	p2, err := s.ListNext(ctx, p.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, p2, true, "c")

	p3, err := s.ListNext(ctx, p2.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, p3, true, "d", "e")

	end, err := s.ListNext(ctx, p3.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, end, false)

	if _, err := s.ListNext(ctx, p.Cursor); !errors.Is(err, ErrUnknownListing) {
		t.Fatalf("pulling a finished listing: err = %v, want ErrUnknownListing", err)
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings still open", n)
	}
}

func TestListSinglePageListingEndsOnTheNextPull(t *testing.T) {
	s := newListService(newPagedFS(ents("a", "b")), time.Minute)
	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, p, true, "a", "b")
	end, err := s.ListNext(context.Background(), p.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, end, false)
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings still open", n)
	}
}

func TestListEmptyDirectory(t *testing.T) {
	s := newListService(newPagedFS(), time.Minute)
	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	wantPage(t, p, false)
}

func TestListErrorMidStreamSurfacesOnListNext(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"))
	fs.err = fmt.Errorf("read /d: %w", vfs.ErrPermission)
	s := newListService(fs, time.Minute)
	ctx := context.Background()

	p, err := s.List(ctx, LocalID, "/d")
	if err != nil {
		t.Fatalf("first page should be delivered: %v", err)
	}
	wantPage(t, p, true, "a")

	p2, err := s.ListNext(ctx, p.Cursor)
	if err != nil {
		t.Fatalf("pages before the failure should be delivered: %v", err)
	}
	wantPage(t, p2, true, "b")

	_, err = s.ListNext(ctx, p2.Cursor)
	if !errors.Is(err, vfs.ErrPermission) {
		t.Fatalf("err = %v, want ErrPermission", err)
	}
	if k := Classify(err); k != ErrKindPermission {
		t.Fatalf("kind = %s, want %s", k, ErrKindPermission)
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings still open after failure", n)
	}
}

func TestListErrorBeforeFirstPageFailsList(t *testing.T) {
	fs := newPagedFS()
	fs.err = vfs.ErrNotExist
	s := newListService(fs, time.Minute)
	if _, err := s.List(context.Background(), LocalID, "/d"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings still open", n)
	}
}

func TestListUnknownConnection(t *testing.T) {
	s := newListService(newPagedFS(), time.Minute)
	_, err := s.List(context.Background(), "ssh:nowhere", "/d")
	if Classify(err) != ErrKindNotConnected {
		t.Fatalf("err = %v, want a not_connected error", err)
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings open", n)
	}
}

func TestListNextUnknownCursor(t *testing.T) {
	s := newListService(newPagedFS(), time.Minute)
	if _, err := s.ListNext(context.Background(), "nope"); !errors.Is(err, ErrUnknownListing) {
		t.Fatalf("err = %v, want ErrUnknownListing", err)
	}
	s.ListClose("nope") // must not panic
}

func TestListCursorsAreUnique(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"))
	s := newListService(fs, time.Minute)
	seen := map[string]bool{}
	for range 50 {
		p, err := s.List(context.Background(), LocalID, "/d")
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Cursor) < 32 || seen[p.Cursor] {
			t.Fatalf("cursor %q is short or repeated", p.Cursor)
		}
		seen[p.Cursor] = true
	}
}

func TestListProducerDoesNotRunAheadOfTheCaller(t *testing.T) {
	pages := make([][]vfs.Entry, 100)
	for i := range pages {
		pages[i] = ents(fmt.Sprint("e", i))
	}
	fs := newPagedFS(pages...)
	s := newListService(fs, time.Minute)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(listWait)
	for fs.produced.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := fs.produced.Load(); n != 2 {
		t.Fatalf("producer made %d pages with 1 pulled, want 2 (the second waits to be pulled)", n)
	}
	s.ListClose(p.Cursor)
}

func TestListCloseStopsTheProducer(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"), ents("c"))
	s := newListService(fs, time.Minute)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	s.ListClose(p.Cursor)
	fs.stopped(t)

	if _, err := s.ListNext(context.Background(), p.Cursor); !errors.Is(err, ErrUnknownListing) {
		t.Fatalf("err = %v, want ErrUnknownListing", err)
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings open", n)
	}
}

func TestListCloseUnblocksAPendingListNext(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"), ents("c"))
	fs.gate = make(chan struct{}) // the producer stalls before page 2
	s := newListService(fs, time.Minute)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	res := make(chan error, 1)
	go func() {
		_, err := s.ListNext(context.Background(), p.Cursor)
		res <- err
	}()
	time.Sleep(20 * time.Millisecond)
	s.ListClose(p.Cursor)

	select {
	case err := <-res:
		if Classify(err) != ErrKindCanceled {
			t.Fatalf("err = %v, want canceled", err)
		}
	case <-time.After(listWait):
		t.Fatal("ListNext still blocked after ListClose")
	}
	fs.stopped(t)
}

func TestCanceledListNextAbandonsTheListing(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"), ents("c"))
	fs.gate = make(chan struct{})
	s := newListService(fs, time.Minute)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan error, 1)
	go func() {
		_, err := s.ListNext(ctx, p.Cursor)
		res <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-res; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	fs.stopped(t)
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings open", n)
	}
}

func TestCanceledFirstListAbandonsTheListing(t *testing.T) {
	fs := newPagedFS(ents("a"))
	fs.start = make(chan struct{}) // List stalls before producing anything
	s := newListService(fs, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan error, 1)
	go func() {
		_, err := s.List(ctx, LocalID, "/d")
		res <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-res; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	fs.stopped(t)
}

func TestIdleListingIsReaped(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"), ents("c"))
	s := newListService(fs, 30*time.Millisecond)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	fs.stopped(t)

	if _, err := s.ListNext(context.Background(), p.Cursor); !errors.Is(err, ErrUnknownListing) {
		t.Fatalf("err = %v, want ErrUnknownListing", err)
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings open", n)
	}
}

func TestSlowPageIsNotReapedWhileACallWaitsForIt(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"), ents("c"))
	fs.gate = make(chan struct{})
	s := newListService(fs, 30*time.Millisecond)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond) // several idle periods
		close(fs.gate)
	}()
	p2, err := s.ListNext(context.Background(), p.Cursor)
	if err != nil {
		t.Fatalf("a listing being pulled must not be reaped: %v", err)
	}
	wantPage(t, p2, true, "b")
}

func TestPullingKeepsAListingAlive(t *testing.T) {
	fs := newPagedFS(ents("a"), ents("b"), ents("c"), ents("d"), ents("e"))
	s := newListService(fs, 150*time.Millisecond)

	p, err := s.List(context.Background(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 { // 3 × 80ms outlives one idle period
		time.Sleep(80 * time.Millisecond)
		if p, err = s.ListNext(context.Background(), p.Cursor); err != nil {
			t.Fatal(err)
		}
	}
	wantPage(t, p, true, "d")
}

func TestConcurrentListingsStayIndependent(t *testing.T) {
	fs := &pagedFS{pages: map[string][][]vfs.Entry{}, returned: make(chan struct{})}
	const n = 8
	for i := range n {
		dir := fmt.Sprint("/d", i)
		for j := range 5 {
			fs.pages[dir] = append(fs.pages[dir], ents(fmt.Sprintf("%d-%d", i, j)))
		}
	}
	s := newListService(fs, time.Minute)

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			var got []string
			p, err := s.List(context.Background(), LocalID, fmt.Sprint("/d", i))
			for err == nil {
				got = append(got, entryNames(p.Entries)...)
				if p.Cursor == "" {
					break
				}
				p, err = s.ListNext(context.Background(), p.Cursor)
			}
			want := fmt.Sprint([]string{fmt.Sprintf("%d-0", i), fmt.Sprintf("%d-1", i), fmt.Sprintf("%d-2", i), fmt.Sprintf("%d-3", i), fmt.Sprintf("%d-4", i)})
			if err != nil || fmt.Sprint(got) != want {
				errs <- fmt.Errorf("dir %d: got %v, err %w", i, got, err)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if open := s.openListings(); open != 0 {
		t.Fatalf("%d listings open", open)
	}
}

func TestAppCloseCancelsListings(t *testing.T) {
	dir := t.TempDir()
	const files = 700 // more than one backend page
	for i := range files {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := New(Config{ConfigDir: t.TempDir(), Emit: func(string, any) {}, Secrets: profiles.Memory()})
	if err != nil {
		t.Fatal(err)
	}
	s := &FileService{app: app}
	ctx := context.Background()

	p, err := s.List(ctx, LocalID, filepath.ToSlash(dir))
	if err != nil {
		t.Fatal(err)
	}
	if p.Cursor == "" {
		t.Fatalf("expected a multi-page listing, got %d entries in one page", len(p.Entries))
	}

	app.Close()

	if _, err := s.ListNext(ctx, p.Cursor); err == nil {
		t.Fatal("ListNext succeeded after App.Close")
	}
	if _, err := s.List(ctx, LocalID, filepath.ToSlash(dir)); err == nil {
		t.Fatal("List succeeded after App.Close")
	}
	if n := s.openListings(); n != 0 {
		t.Fatalf("%d listings open after Close", n)
	}
}

func TestListLocalDirectoryDeliversEveryEntryOnce(t *testing.T) {
	dir := t.TempDir()
	var want []string
	for i := range 600 {
		name := fmt.Sprintf("f%04d", i)
		want = append(want, name)
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := New(Config{ConfigDir: t.TempDir(), Emit: func(string, any) {}, Secrets: profiles.Memory()})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	s := &FileService{app: app}

	var got []string
	pages := 0
	p, err := s.List(context.Background(), LocalID, filepath.ToSlash(dir))
	for err == nil {
		pages++
		got = append(got, entryNames(p.Entries)...)
		if p.Cursor == "" {
			break
		}
		p, err = s.ListNext(context.Background(), p.Cursor)
	}
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if pages < 2 || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("pages = %d, got %d entries, want %d", pages, len(got), len(want))
	}
}
