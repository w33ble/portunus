package preview

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestForNamePicksKindByExtension(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		kind Kind
		ct   string
	}{
		{"notes.txt", KindText, TextContentType},
		{"main.go", KindText, TextContentType},
		{"index.html", KindText, TextContentType},
		{"Makefile", KindText, TextContentType},
		{"archive.tar.gz", KindText, TextContentType},
		{"photo.JPG", KindImage, "image/jpeg"},
		{"icon.svg", KindImage, "image/svg+xml"},
		{"shot.webp", KindImage, "image/webp"},
		{"report.pdf", KindPDF, "application/pdf"},
		{"REPORT.PDF", KindPDF, "application/pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := ForName(c.name)
			if p.Kind != c.kind || p.ContentType != c.ct {
				t.Fatalf("ForName(%q) = %s %q, want %s %q", c.name, p.Kind, p.ContentType, c.kind, c.ct)
			}
		})
	}
}

func TestAcceptsSniffsTextContent(t *testing.T) {
	t.Parallel()
	latin1 := []byte("caf\xe9 au lait")
	cases := []struct {
		name string
		file string
		head []byte
		want bool
	}{
		{"utf-8 without an extension", "README", []byte("héllo wörld\n"), true},
		{"NUL byte", "data", []byte("abc\x00def"), false},
		{"NUL byte in a .txt", "notes.txt", []byte("abc\x00def"), false},
		{"invalid UTF-8 without a text extension", "blob", latin1, false},
		{"invalid UTF-8 in a .log", "app.log", latin1, true},
		{"rune cut off by the sniff window", "README", []byte("ab\xe2\x82"), true},
		{"empty file", "empty", nil, true},
		{"images are taken on their extension", "x.png", []byte("\x89PNG\x00\x00"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := ForName(c.file).Accepts(c.head); got != c.want {
				t.Fatalf("Accepts(%q) = %v, want %v", c.head, got, c.want)
			}
		})
	}
}

func TestTooLargeAppliesLimitsToImagesAndPDFsOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		size int64
		want bool
	}{
		{"a.png", ImageLimit, false},
		{"a.png", ImageLimit + 1, true},
		{"a.pdf", PDFLimit, false},
		{"a.pdf", PDFLimit + 1, true},
		{"a.log", 12 << 30, false},
	}
	for _, c := range cases {
		if got := ForName(c.name).TooLarge(c.size); got != c.want {
			t.Errorf("%s at %d bytes: TooLarge = %v, want %v", c.name, c.size, got, c.want)
		}
	}
}

// countingReader records how many bytes were read from it.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestReadText(t *testing.T) {
	t.Parallel()
	limit := TextLimit
	cases := []struct {
		name      string
		content   string
		wantLen   int64
		truncated bool
	}{
		{"short file", "hello\n", 6, false},
		{"exactly the limit", strings.Repeat("a", int(limit)), limit, false},
		{"one byte past the limit", strings.Repeat("a", int(limit)+1), limit, true},
		// "€" is 3 bytes; the limit falls inside the last one, which is dropped.
		{"limit inside a rune", strings.Repeat("a", int(limit)-1) + "€€", limit - 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			text, ok, err := ReadText(strings.NewReader(c.content), ForName("f.txt"), int64(len(c.content)))
			if err != nil || !ok {
				t.Fatalf("ReadText: ok=%v err=%v", ok, err)
			}
			if int64(len(text.Data)) != c.wantLen || text.Truncated != c.truncated {
				t.Fatalf("got %d bytes truncated=%v, want %d truncated=%v", len(text.Data), text.Truncated, c.wantLen, c.truncated)
			}
			if !bytes.HasPrefix([]byte(c.content), text.Data) {
				t.Fatal("data is not a prefix of the file")
			}
		})
	}
}

func TestReadTextTruncatesAFileThatGrewPastItsStatSize(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("x", int(TextLimit)+100)
	text, ok, err := ReadText(strings.NewReader(content), ForName("grow.log"), 10)
	if err != nil || !ok {
		t.Fatalf("ReadText: ok=%v err=%v", ok, err)
	}
	if int64(len(text.Data)) != TextLimit || !text.Truncated {
		t.Fatalf("got %d bytes truncated=%v, want %d truncated", len(text.Data), text.Truncated, TextLimit)
	}
}

func TestReadTextStopsAfterTheSniffWindowForBinaries(t *testing.T) {
	t.Parallel()
	binary := bytes.Repeat([]byte{0, 1, 2, 3}, 1<<20)
	r := &countingReader{r: bytes.NewReader(binary)}
	_, ok, err := ReadText(r, ForName("random.bin"), int64(len(binary)))
	if err != nil || ok {
		t.Fatalf("ReadText: ok=%v err=%v, want not ok", ok, err)
	}
	if r.n > SniffLen {
		t.Fatalf("read %d bytes of a binary, want at most %d", r.n, SniffLen)
	}
}
