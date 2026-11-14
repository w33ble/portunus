package services

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/w33ble/portunus/internal/preview"
	"github.com/w33ble/portunus/internal/transfer"
	"github.com/w33ble/portunus/internal/vfs"
)

// PreviewRoute is where the asset server mounts PreviewService.
const PreviewRoute = "/preview"

// Preview response headers. A file with nothing to show answers 204 with a reason.
const (
	HeaderPreviewKind      = "X-Preview-Kind"
	HeaderPreviewTruncated = "X-Preview-Truncated"
	HeaderPreviewReason    = "X-Preview-Reason"
)

// Reasons a file has no preview.
const (
	ReasonBinary   = "binary"
	ReasonTooLarge = "too_large"
)

// errIsDir rejects preview requests for directories; the frontend summarizes those instead.
var errIsDir = errors.New("is a directory")

const previewCopyBuf = 1 << 20

// PreviewService serves the start of a file for the quick preview at
// GET PreviewRoute?conn=<connId>&path=<path>. It is an http.Handler on the asset server, so
// <img> and <iframe> load it directly and binary content never goes through JSON bindings.
// Reads stop when the request is canceled: the webview cancels it when the preview moves on.
type PreviewService struct {
	app *App
}

func (s *PreviewService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writePreviewError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	ctx := r.Context()
	q := r.URL.Query()
	p := q.Get("path")
	if err := transfer.CheckPath(p); err != nil {
		writePreviewError(w, http.StatusBadRequest, err)
		return
	}
	// Only connections the registry already knows: a preview never dials a new target.
	fsys, err := s.app.reg.FS(ctx, q.Get("conn"))
	if err != nil {
		writeClassifiedError(w, err)
		return
	}
	e, err := fsys.Stat(ctx, p)
	if err != nil {
		writeClassifiedError(w, err)
		return
	}
	if e.IsDir {
		writePreviewError(w, http.StatusBadRequest, fmt.Errorf("%s: %w", p, errIsDir))
		return
	}

	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")

	plan := preview.ForName(e.Name)
	if plan.TooLarge(e.Size) {
		noPreview(w, ReasonTooLarge)
		return
	}

	rc, err := fsys.Open(ctx, p, 0)
	if err != nil {
		writeClassifiedError(w, err)
		return
	}
	// Closing aborts in-flight reads on backends that ignore ctx (SFTP), so a canceled
	// request stops downloading at once.
	stop := context.AfterFunc(ctx, func() { _ = rc.Close() })
	defer func() {
		if stop() {
			_ = rc.Close()
		}
	}()

	if plan.Kind == preview.KindText {
		serveText(w, r, rc, plan, e)
		return
	}

	setContentHeaders(h, plan)
	h.Set("Content-Length", strconv.FormatInt(e.Size, 10))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.CopyBuffer(w, io.LimitReader(rc, e.Size), make([]byte, previewCopyBuf)); err != nil && ctx.Err() == nil {
		s.app.log.WarnContext(ctx, "preview stream failed", "path", p, "error", err)
	}
}

func serveText(w http.ResponseWriter, r *http.Request, rc io.Reader, plan preview.Plan, e vfs.Entry) {
	text, ok, err := preview.ReadText(rc, plan, e.Size)
	if err != nil {
		writeClassifiedError(w, err)
		return
	}
	if !ok {
		noPreview(w, ReasonBinary)
		return
	}

	h := w.Header()
	setContentHeaders(h, plan)
	h.Set("Content-Length", strconv.Itoa(len(text.Data)))
	if text.Truncated {
		h.Set(HeaderPreviewTruncated, "true")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(text.Data)
	}
}

func setContentHeaders(h http.Header, plan preview.Plan) {
	h.Set(HeaderPreviewKind, string(plan.Kind))
	h.Set("Content-Type", plan.ContentType)
	h.Set("Content-Disposition", "inline")
	// The PDF viewer can't run in a sandbox; text and images never need scripts.
	if plan.Kind != preview.KindPDF {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
}

func noPreview(w http.ResponseWriter, reason string) {
	w.Header().Set(HeaderPreviewReason, reason)
	w.WriteHeader(http.StatusNoContent)
}

func writeClassifiedError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch Classify(err) {
	case ErrKindNotExist, ErrKindNotConnected:
		status = http.StatusNotFound
	case ErrKindPermission:
		status = http.StatusForbidden
	case ErrKindCanceled:
		// The client is gone or gave up; nobody reads this.
		status = http.StatusServiceUnavailable
	case ErrKindUnsupported:
		status = http.StatusUnsupportedMediaType
	case ErrKindExist, ErrKindHostKeyMismatch, ErrKindOther:
	}
	writePreviewError(w, status, err)
}

// writePreviewError answers with the same {kind,message} body binding calls reject with.
func writePreviewError(w http.ResponseWriter, status int, err error) {
	body, jerr := json.Marshal(ErrorCause{Kind: Classify(err), Message: err.Error()})
	if jerr != nil {
		body = []byte(`{"kind":"other","message":"preview failed"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
