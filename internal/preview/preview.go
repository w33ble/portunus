// Package preview decides how the quick preview shows a file and how much of it to read.
//
// The file name picks a plan; for text, the first SniffLen bytes then confirm the content
// really is text. Nothing here touches a filesystem: callers hand over an io.Reader.
package preview

import (
	"bytes"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Kind is how the frontend renders a preview.
type Kind string

const (
	KindText  Kind = "text"
	KindImage Kind = "image"
	KindPDF   Kind = "pdf"
)

const (
	// TextLimit is how much of a text file is shown; longer files are truncated.
	TextLimit int64 = 1 << 20
	// ImageLimit and PDFLimit are the largest files previewed at all.
	ImageLimit int64 = 20 << 20
	PDFLimit   int64 = 50 << 20
	// SniffLen is how much of a file decides whether it is text.
	SniffLen = 8 << 10
)

// TextContentType is served for every text preview, HTML included, so nothing is rendered.
const TextContentType = "text/plain; charset=utf-8"

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml",
}

// textExts name files that are text even when they aren't valid UTF-8 (Latin-1 logs, say).
var textExts = map[string]bool{
	".txt": true, ".text": true, ".md": true, ".markdown": true, ".rst": true, ".log": true,
	".json": true, ".jsonl": true, ".ndjson": true, ".yaml": true, ".yml": true, ".toml": true,
	".ini": true, ".conf": true, ".cfg": true, ".env": true, ".properties": true,
	".csv": true, ".tsv": true, ".xml": true, ".html": true, ".htm": true, ".css": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true, ".bat": true,
	".go": true, ".mod": true, ".sum": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".mjs": true, ".cjs": true, ".py": true, ".rb": true, ".rs": true, ".c": true, ".h": true,
	".cc": true, ".cpp": true, ".hpp": true, ".java": true, ".kt": true, ".swift": true,
	".php": true, ".pl": true, ".lua": true, ".sql": true, ".diff": true, ".patch": true,
}

// Plan is how a file is previewed, decided from its name before any content is read.
type Plan struct {
	Kind        Kind
	ContentType string
	// Limit is the largest file shown for images and PDFs, and how much is shown for text.
	Limit int64
	// named is true when the extension says text, which relaxes sniffing to "no NUL bytes".
	named bool
}

// ForName plans the preview of a file called name. Unknown names plan as text, and
// sniffing (Accepts) decides whether they really are.
func ForName(name string) Plan {
	ext := strings.ToLower(path.Ext(name))
	if ct, ok := imageTypes[ext]; ok {
		return Plan{Kind: KindImage, ContentType: ct, Limit: ImageLimit}
	}
	if ext == ".pdf" {
		return Plan{Kind: KindPDF, ContentType: "application/pdf", Limit: PDFLimit}
	}
	return Plan{Kind: KindText, ContentType: TextContentType, Limit: TextLimit, named: textExts[ext]}
}

// TooLarge reports whether a file of size is past the plan's limit. Text is never too
// large: it is truncated instead.
func (p Plan) TooLarge(size int64) bool {
	return p.Kind != KindText && size > p.Limit
}

// Accepts reports whether head, the start of the file, fits the plan. Text must have no
// NUL bytes and, unless the name says text, be valid UTF-8 (a rune cut off at the end of
// head is fine). Images and PDFs are taken on their extension.
func (p Plan) Accepts(head []byte) bool {
	if p.Kind != KindText {
		return true
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	return p.named || utf8.Valid(trimPartialRune(head))
}

// Text is the part of a text file a preview shows.
type Text struct {
	Data []byte
	// Truncated is true when the file had more than Data; Data then ends on a whole rune.
	Truncated bool
}

// ReadText reads at most p.Limit bytes of r for a text preview; size, the file's stat size,
// only sizes the buffer. ok is false when the first SniffLen bytes aren't text, and
// nothing past them is read.
func ReadText(r io.Reader, p Plan, size int64) (text Text, ok bool, err error) {
	var buf bytes.Buffer
	// One byte past the limit tells a truncated file from one that is exactly Limit long;
	// MinRead covers ReadFrom's spare capacity so the buffer never regrows.
	buf.Grow(int(min(max(size, SniffLen), p.Limit)) + 1 + bytes.MinRead)

	if _, err := buf.ReadFrom(io.LimitReader(r, SniffLen)); err != nil {
		return Text{}, false, err
	}
	if !p.Accepts(buf.Bytes()) {
		return Text{}, false, nil
	}
	if _, err := buf.ReadFrom(io.LimitReader(r, p.Limit+1-int64(buf.Len()))); err != nil {
		return Text{}, false, err
	}

	data := buf.Bytes()
	if int64(len(data)) <= p.Limit {
		return Text{Data: data}, true, nil
	}
	return Text{Data: trimPartialRune(data[:p.Limit]), Truncated: true}, true, nil
}

// trimPartialRune drops an incomplete UTF-8 sequence from the end of b.
func trimPartialRune(b []byte) []byte {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i]
			}
			break
		}
	}
	return b
}
