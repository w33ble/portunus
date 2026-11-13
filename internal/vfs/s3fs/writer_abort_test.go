package s3fs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

// An Abort that lands while CreateMultipartUpload is in flight must still abort the upload the
// server creates; otherwise it's orphaned (and billed) with no ID left to clean it up.
func TestAbortDuringCreateStillAbortsUpload(t *testing.T) {
	var (
		mu        sync.Mutex
		aborted   []string
		started   = make(chan struct{})
		startOnce sync.Once
		release   = make(chan struct{})
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && q.Has("uploads"):
			startOnce.Do(func() { close(started) })
			<-release // hold the request until Abort has run
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<InitiateMultipartUploadResult><Bucket>b</Bucket><Key>k</Key><UploadId>up-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodDelete && q.Has("uploadId"):
			mu.Lock()
			aborted = append(aborted, q.Get("uploadId"))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.String(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	f, err := New(ctx, Config{
		AccessKeyID: "k", SecretAccessKey: "s", Region: "us-east-1",
		Endpoint: srv.URL, PathStyle: true, Bucket: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.Create(ctx, "/b/k", vfs.CreateOpts{Size: 64 << 20})
	if err != nil {
		t.Fatal(err)
	}

	// More than one part, so Write starts the multipart upload.
	writeErr := make(chan error, 1)
	go func() {
		_, err := w.Write(make([]byte, 9<<20))
		writeErr <- err
	}()

	<-started
	if err := w.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	close(release)
	if err := <-writeErr; err == nil {
		t.Fatal("Write succeeded after Abort")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		got := slices.Clone(aborted)
		mu.Unlock()
		if slices.Contains(got, "up-1") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("upload up-1 was never aborted (aborts: %v)", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
