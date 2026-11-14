package services

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/preview"
	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/localfs"
)

func newPreviewService(local vfs.FS) *PreviewService {
	reg := newRegistry(local, nil, func([]Connection) {})
	return &PreviewService{app: &App{reg: reg, log: slog.New(slog.DiscardHandler)}}
}

func previewRequest(ctx context.Context, method, conn, p string) *http.Request {
	q := url.Values{"conn": {conn}, "path": {p}}
	return httptest.NewRequestWithContext(ctx, method, PreviewRoute+"?"+q.Encode(), nil)
}

// slash turns a temp path into the '/'-separated form vfs paths use on every OS.
func slash(p string) string { return filepath.ToSlash(p) }

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return slash(p)
}

func errorKind(t *testing.T, body []byte) ErrorKind {
	t.Helper()
	var cause ErrorCause
	if err := json.Unmarshal(body, &cause); err != nil {
		t.Fatalf("error body %q: %v", body, err)
	}
	return cause.Kind
}

func TestPreviewRejectsBadRequests(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := writeFile(t, dir, "a.txt", []byte("a"))
	svc := newPreviewService(localfs.New())

	cases := []struct {
		name   string
		method string
		conn   string
		path   string
		status int
		kind   ErrorKind
	}{
		{"unknown connection", http.MethodGet, "ssh:nowhere", file, http.StatusNotFound, ErrKindNotConnected},
		{"empty connection", http.MethodGet, "", file, http.StatusNotFound, ErrKindNotConnected},
		{"empty path", http.MethodGet, LocalID, "", http.StatusBadRequest, ErrKindOther},
		{"dot-dot segment", http.MethodGet, LocalID, slash(dir) + "/x/../a.txt", http.StatusBadRequest, ErrKindOther},
		{"trailing slash", http.MethodGet, LocalID, file + "/", http.StatusBadRequest, ErrKindOther},
		{"doubled slash", http.MethodGet, LocalID, slash(dir) + "//a.txt", http.StatusBadRequest, ErrKindOther},
		{"root", http.MethodGet, LocalID, "/", http.StatusBadRequest, ErrKindOther},
		{"directory", http.MethodGet, LocalID, slash(dir), http.StatusBadRequest, ErrKindOther},
		{"missing file", http.MethodGet, LocalID, slash(dir) + "/missing.txt", http.StatusNotFound, ErrKindNotExist},
		{"POST", http.MethodPost, LocalID, file, http.StatusMethodNotAllowed, ErrKindOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			svc.ServeHTTP(rec, previewRequest(t.Context(), c.method, c.conn, c.path))
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d (%s)", rec.Code, c.status, rec.Body)
			}
			if kind := errorKind(t, rec.Body.Bytes()); kind != c.kind {
				t.Fatalf("kind %q, want %q", kind, c.kind)
			}
		})
	}
}

func TestPreviewServesFilesByKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0, 7}, 5000)...)
	pdf := []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	files := map[string][]byte{
		"hello.txt":  []byte("hello from portunus\n"),
		"page.html":  []byte("<script>alert(1)</script>"),
		"pixel.png":  png,
		"doc.pdf":    pdf,
		"random.bin": append([]byte("ELF\x00"), bytes.Repeat([]byte{1}, 100)...),
	}
	for name, data := range files {
		writeFile(t, dir, name, data)
	}
	svc := newPreviewService(localfs.New())

	cases := []struct {
		file   string
		status int
		kind   string
		ct     string
		reason string
	}{
		{"hello.txt", http.StatusOK, "text", preview.TextContentType, ""},
		{"page.html", http.StatusOK, "text", preview.TextContentType, ""},
		{"pixel.png", http.StatusOK, "image", "image/png", ""},
		{"doc.pdf", http.StatusOK, "pdf", "application/pdf", ""},
		{"random.bin", http.StatusNoContent, "", "", ReasonBinary},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			svc.ServeHTTP(rec, previewRequest(t.Context(), http.MethodGet, LocalID, slash(dir)+"/"+c.file))
			h := rec.Header()
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d (%s)", rec.Code, c.status, rec.Body)
			}
			if h.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing X-Content-Type-Options: nosniff")
			}
			if got := h.Get(HeaderPreviewReason); got != c.reason {
				t.Fatalf("reason %q, want %q", got, c.reason)
			}
			if c.status != http.StatusOK {
				if rec.Body.Len() != 0 {
					t.Fatalf("no-preview response has a body: %q", rec.Body)
				}
				return
			}
			if h.Get(HeaderPreviewKind) != c.kind || h.Get("Content-Type") != c.ct {
				t.Fatalf("kind %q type %q, want %q %q", h.Get(HeaderPreviewKind), h.Get("Content-Type"), c.kind, c.ct)
			}
			if h.Get("Content-Disposition") != "inline" {
				t.Fatalf("Content-Disposition %q, want inline", h.Get("Content-Disposition"))
			}
			if !bytes.Equal(rec.Body.Bytes(), files[c.file]) {
				t.Fatalf("body differs from the file (%d vs %d bytes)", rec.Body.Len(), len(files[c.file]))
			}
		})
	}
}

func TestPreviewTruncatesLongText(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writeFile(t, dir, "big.log", bytes.Repeat([]byte("line\n"), int(preview.TextLimit)))

	rec := httptest.NewRecorder()
	newPreviewService(localfs.New()).ServeHTTP(rec, previewRequest(t.Context(), http.MethodGet, LocalID, p))
	if rec.Code != http.StatusOK || rec.Header().Get(HeaderPreviewTruncated) != "true" {
		t.Fatalf("status %d truncated %q, want 200 true", rec.Code, rec.Header().Get(HeaderPreviewTruncated))
	}
	if int64(rec.Body.Len()) != preview.TextLimit {
		t.Fatalf("body %d bytes, want %d", rec.Body.Len(), preview.TextLimit)
	}
}

func TestPreviewRefusesImagesPastTheLimitWithoutReading(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse: the size is all that matters.
	if err := f.Truncate(preview.ImageLimit + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	rec := httptest.NewRecorder()
	newPreviewService(localfs.New()).ServeHTTP(rec, previewRequest(t.Context(), http.MethodGet, LocalID, slash(p)))
	if rec.Code != http.StatusNoContent || rec.Header().Get(HeaderPreviewReason) != ReasonTooLarge {
		t.Fatalf("status %d reason %q, want 204 %s", rec.Code, rec.Header().Get(HeaderPreviewReason), ReasonTooLarge)
	}
}

func TestPreviewHeadAnswersWithoutABody(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writeFile(t, dir, "doc.pdf", []byte("%PDF-1.4\n"))

	rec := httptest.NewRecorder()
	newPreviewService(localfs.New()).ServeHTTP(rec, previewRequest(t.Context(), http.MethodHead, LocalID, p))
	if rec.Code != http.StatusOK || rec.Header().Get(HeaderPreviewKind) != "pdf" {
		t.Fatalf("status %d kind %q, want 200 pdf", rec.Code, rec.Header().Get(HeaderPreviewKind))
	}
	if rec.Header().Get("Content-Length") != "9" || rec.Body.Len() != 0 {
		t.Fatalf("Content-Length %q body %d bytes, want 9 and empty", rec.Header().Get("Content-Length"), rec.Body.Len())
	}
}

// stallFS serves one image whose reader delivers a first chunk, then blocks until closed,
// like an SFTP read that ignores ctx.
type stallFS struct {
	vfs.FS
	closed chan struct{}
}

func (f *stallFS) Home(context.Context) (string, error) { return "/", nil }

func (f *stallFS) Stat(_ context.Context, p string) (vfs.Entry, error) {
	return vfs.Entry{Name: path.Base(p), Path: p, Size: 1 << 20, Mode: 0o644}, nil
}

func (f *stallFS) Open(context.Context, string, int64) (io.ReadCloser, error) {
	return &stallReader{closed: f.closed}, nil
}

type stallReader struct {
	closed chan struct{}
	sent   bool
}

func (r *stallReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, "\x89PNG"), nil
	}
	<-r.closed
	return 0, fs.ErrClosed
}

func (r *stallReader) Close() error { close(r.closed); return nil }

func TestPreviewCancelClosesTheReader(t *testing.T) {
	t.Parallel()
	fsys := &stallFS{closed: make(chan struct{})}
	svc := newPreviewService(fsys)

	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan struct{})
	go func() {
		defer close(served)
		svc.ServeHTTP(httptest.NewRecorder(), previewRequest(ctx, http.MethodGet, LocalID, "/photos/big.png"))
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("handler still reading after the request was canceled")
	}
	select {
	case <-fsys.closed:
	default:
		t.Fatal("reader not closed")
	}
}

func TestSummarizeCountsImmediateEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", []byte("12345"))
	writeFile(t, dir, "b.txt", []byte("123"))
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub"), "ignored.txt", []byte(strings.Repeat("x", 100)))

	svc := &FileService{app: &App{reg: newRegistry(localfs.New(), nil, func([]Connection) {})}}
	got, err := svc.Summarize(t.Context(), LocalID, slash(dir))
	if err != nil {
		t.Fatal(err)
	}
	if want := (DirSummary{Dirs: 1, Files: 2, Size: 8}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSummarizeStopsAtTheLimit(t *testing.T) {
	t.Parallel()
	page := make([]vfs.Entry, summaryLimit+5)
	for i := range page {
		page[i] = vfs.Entry{Name: "f", Size: 1}
	}
	fsys := newPagedFS(page)

	svc := &FileService{app: &App{reg: newRegistry(fsys, nil, func([]Connection) {})}}
	got, err := svc.Summarize(t.Context(), LocalID, "/d")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Partial || got.Files != summaryLimit {
		t.Fatalf("got %+v, want Partial with %d files", got, summaryLimit)
	}
}
