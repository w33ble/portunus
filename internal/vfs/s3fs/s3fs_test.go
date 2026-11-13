package s3fs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/vfstest"
)

func TestConformance(t *testing.T) {
	if minioEndpoint == "" {
		t.Skip("MinIO via docker unavailable")
	}
	vfstest.Run(t, vfstest.Harness{
		New: func(t *testing.T) (vfs.FS, string) {
			f, bucket := newBucket(t, minioConfig())
			root := bucket + "/work"
			if err := f.Mkdir(context.Background(), root); err != nil {
				t.Fatal(err)
			}
			return f, root
		},
		Caps: vfstest.Caps{EmptyDirs: true},
	})
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	r := rand.NewChaCha8([32]byte{1, 2, 3})
	_, _ = r.Read(b)
	return b
}

func put(t *testing.T, f *FS, p string, data []byte) {
	t.Helper()
	w, err := f.Create(context.Background(), p, vfs.CreateOpts{Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
}

func readAll(t *testing.T, f *FS, p string) []byte {
	t.Helper()
	r, err := f.Open(context.Background(), p, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func pendingUploads(t *testing.T, f *FS, bucket string) int {
	t.Helper()
	out, err := f.client(f.cfg.Region).ListMultipartUploads(context.Background(), &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatal(err)
	}
	return len(out.Uploads)
}

func TestMultipartUploadRoundTrip(t *testing.T) {
	cfg := minioConfig()
	cfg.PartSize = 5 << 20
	f, root := newBucket(t, cfg)
	data := randBytes(5<<20*3 + 12345) // four parts, last one short
	w, err := f.Create(context.Background(), root+"/big.bin", vfs.CreateOpts{Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	// ReadFrom path (what io.Copy uses) with an awkward reader.
	n, err := io.Copy(w, io.MultiReader(bytes.NewReader(data[:7<<20]), bytes.NewReader(data[7<<20:])))
	if err != nil || n != int64(len(data)) {
		t.Fatalf("copy = %d, %v", n, err)
	}
	if _, visible := f.Stat(context.Background(), root+"/big.bin"); visible == nil {
		t.Fatal("object visible before Commit")
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, f, root+"/big.bin"); sha256.Sum256(got) != sha256.Sum256(data) {
		t.Fatal("content mismatch after multipart upload")
	}
	if n := pendingUploads(t, f, root[1:]); n != 0 {
		t.Fatalf("%d uploads still pending", n)
	}
}

func TestMultipartExactPartMultiple(t *testing.T) {
	cfg := minioConfig()
	cfg.PartSize = 5 << 20
	f, root := newBucket(t, cfg)
	data := randBytes(10 << 20)
	put(t, f, root+"/exact", data)
	if !bytes.Equal(readAll(t, f, root+"/exact"), data) {
		t.Fatal("content mismatch")
	}
}

func TestMultipartAbortLeavesNothing(t *testing.T) {
	cfg := minioConfig()
	cfg.PartSize = 5 << 20
	f, root := newBucket(t, cfg)
	size := 12 << 20
	w, err := f.Create(context.Background(), root+"/aborted", vfs.CreateOpts{Size: int64(size)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(randBytes(11 << 20)); err != nil {
		t.Fatal(err)
	}
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Stat(context.Background(), root+"/aborted"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("Stat after abort = %v", err)
	}
	if n := pendingUploads(t, f, root[1:]); n != 0 {
		t.Fatalf("%d uploads still pending after Abort", n)
	}
}

func TestSizeMismatchAbortsMultipart(t *testing.T) {
	cfg := minioConfig()
	cfg.PartSize = 5 << 20
	f, root := newBucket(t, cfg)
	w, err := f.Create(context.Background(), root+"/short", vfs.CreateOpts{Size: 20 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(randBytes(6 << 20)); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); !errors.Is(err, vfs.ErrSizeMismatch) {
		t.Fatalf("Commit = %v", err)
	}
	if n := pendingUploads(t, f, root[1:]); n != 0 {
		t.Fatalf("%d uploads pending", n)
	}
	if _, err := f.Stat(context.Background(), root+"/short"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("Stat = %v", err)
	}
}

func TestRootListsBucketsAndFixedBucketSkipsListing(t *testing.T) {
	f, root := newBucket(t, minioConfig())
	var names []string
	err := f.List(context.Background(), "/", func(es []vfs.Entry) error {
		for _, e := range es {
			if !e.IsDir {
				t.Errorf("bucket %s not a dir", e.Name)
			}
			names = append(names, e.Path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(names, root) {
		t.Fatalf("%s missing from %v", root, names)
	}

	// A fixed bucket must list without ListBuckets (credentials here can't even list).
	cfg := minioConfig()
	cfg.Bucket = root[1:]
	fixed := newMinioFS(t, cfg)
	var got []string
	if err := fixed.List(context.Background(), "/", func(es []vfs.Entry) error {
		for _, e := range es {
			got = append(got, e.Path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != root {
		t.Fatalf("fixed list = %v", got)
	}
	if home, _ := fixed.Home(context.Background()); home != root {
		t.Fatalf("Home = %s", home)
	}
	if _, err := fixed.Stat(context.Background(), "/other-bucket/x"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("other bucket err = %v", err)
	}
}

func contains(s []string, v string) bool { return slices.Contains(s, v) }

func TestRenameObjectAndPrefix(t *testing.T) {
	f, root := newBucket(t, minioConfig())
	ctx := context.Background()
	put(t, f, root+"/a b+c.txt", []byte("hello"))
	if err := f.Rename(ctx, root+"/a b+c.txt", root+"/moved/x é.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, f, root+"/moved/x é.txt"); string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	if _, err := f.Stat(ctx, root+"/a b+c.txt"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("source remains: %v", err)
	}
	put(t, f, root+"/other", []byte("o"))
	if err := f.Rename(ctx, root+"/other", root+"/moved/x é.txt"); !errors.Is(err, vfs.ErrExist) {
		t.Fatalf("rename onto existing = %v", err)
	}
	if err := f.Rename(ctx, root+"/moved", root+"/moved2"); !errors.Is(err, vfs.ErrUnsupported) {
		t.Fatalf("rename prefix = %v", err)
	}
}

func TestRemoveNonEmptyPrefixFails(t *testing.T) {
	f, root := newBucket(t, minioConfig())
	ctx := context.Background()
	if err := f.Mkdir(ctx, root+"/d"); err != nil {
		t.Fatal(err)
	}
	put(t, f, root+"/d/f", []byte("x"))
	if err := f.Remove(ctx, root+"/d"); err == nil || errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("Remove non-empty = %v", err)
	}
	if err := f.Remove(ctx, root+"/d/f"); err != nil {
		t.Fatal(err)
	}
	// The marker keeps the directory alive until it is removed itself.
	if e, err := f.Stat(ctx, root+"/d"); err != nil || !e.IsDir {
		t.Fatalf("dir gone early: %+v %v", e, err)
	}
	if err := f.Remove(ctx, root+"/d"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Stat(ctx, root+"/d"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("Stat = %v", err)
	}
}

func TestMkdirOverFileFails(t *testing.T) {
	f, root := newBucket(t, minioConfig())
	put(t, f, root+"/file", []byte("x"))
	if err := f.Mkdir(context.Background(), root+"/file"); !errors.Is(err, vfs.ErrExist) {
		t.Fatalf("Mkdir over file = %v", err)
	}
}

func TestOpenCloseCancelsRead(t *testing.T) {
	f, root := newBucket(t, minioConfig())
	put(t, f, root+"/f", randBytes(1<<20))
	r, err := f.Open(context.Background(), root+"/f", 1<<20) // offset == size
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	if len(b) != 0 {
		t.Fatalf("read %d bytes at EOF offset", len(b))
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPartSizeFor(t *testing.T) {
	cases := []struct {
		size, configured, want int64
	}{
		{0, 0, 8 << 20},
		{100, 1, 5 << 20},
		{100, 16 << 20, 16 << 20},
		{100 << 30, 0, 11 << 20}, // 100 GiB / 10000 rounded up to MiB
		{5 << 40, 0, 525 << 20},  // 5 TiB
		{1 << 60, 0, 5 << 30},    // clamp at 5 GiB
	}
	for _, c := range cases {
		got := partSizeFor(c.size, c.configured)
		if got != c.want {
			t.Errorf("partSizeFor(%d,%d) = %d, want %d", c.size, c.configured, got, c.want)
		}
		if c.size > 0 && c.size <= maxParts*maxPartSize && (c.size+got-1)/got > maxParts {
			t.Errorf("partSizeFor(%d) = %d exceeds %d parts", c.size, got, maxParts)
		}
	}
	if n := bufferCount(5 << 30); n != 1 {
		t.Errorf("bufferCount(5GiB) = %d, want 1", n)
	}
	if n := bufferCount(8 << 20); n != maxParallel+1 {
		t.Errorf("bufferCount(8MiB) = %d", n)
	}
}

func TestParse(t *testing.T) {
	f := &FS{}
	for in, want := range map[string]loc{
		"/":         {},
		"/b":        {"b", ""},
		"/b/":       {"b", ""},
		"/b/k/../j": {"b", "j"},
		"/b/a/b/c":  {"b", "a/b/c"},
	} {
		got, err := f.parse(in)
		if err != nil || got != want {
			t.Errorf("parse(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	if _, err := f.parse("rel"); err == nil {
		t.Error("relative path accepted")
	}
}
