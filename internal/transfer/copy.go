package transfer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

// removeCopiedTimeout bounds deleting a source whose copy has already committed.
const removeCopiedTimeout = 10 * time.Second

// xfer owns the two ends of one file copy. Cancellation closes the reader and aborts the
// writer from another goroutine, which is what stops backends that ignore ctx; the once
// flags make that safe against the copy goroutine doing the same on its way out.
type xfer struct {
	mu       sync.Mutex
	r        io.ReadCloser
	w        vfs.Writer
	rClosed  bool
	wSettled bool // Commit or Abort has been called
	abortErr error
}

func (x *xfer) closeReader() {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.rClosed {
		x.rClosed = true
		_ = x.r.Close()
	}
}

func (x *xfer) abort() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.wSettled {
		return nil
	}
	x.wSettled = true
	err := x.w.Abort()
	if err != nil {
		x.abortErr = err
	}
	return err
}

func (x *xfer) abortFailure() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.abortErr
}

// cancel runs when the job's context ends.
func (x *xfer) cancel() {
	x.closeReader()
	_ = x.abort()
}

// beginCommit claims the writer for Commit; false means cancellation got there first.
func (x *xfer) beginCommit() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.wSettled {
		return false
	}
	x.wSettled = true
	return true
}

type countWriter struct {
	w   io.Writer
	add func(int64)
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.add(int64(n))
	}
	return n, err
}

type countReader struct {
	r   io.Reader
	add func(int64)
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.add(int64(n))
	}
	return n, err
}

func (e *Engine) copyFile(j *job, t *task) error {
	ctx := j.ctx
	r, err := j.src.Open(ctx, t.src, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", t.src, err)
	}
	w, err := j.dst.Create(ctx, t.dst, vfs.CreateOpts{Size: t.size, Mode: t.ent.Mode.Perm(), ModTime: t.ent.ModTime})
	if err != nil {
		_ = r.Close()
		return fmt.Errorf("create %s: %w", t.dst, err)
	}
	x := &xfer{r: r, w: w}
	stop := context.AfterFunc(ctx, x.cancel)
	defer stop()
	defer func() {
		if aerr := x.abortFailure(); aerr != nil {
			e.log.WarnContext(ctx, "transfer abort failed", "job", j.id, "dst", t.dst, "err", aerr)
		}
	}()

	add := func(n int64) {
		t.done.Add(n)
		j.doneBytes.Add(n)
		e.touch(j)
	}
	if err := e.pump(x, add); err != nil {
		x.cancel()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("copy %s: %w", t.src, err)
	}
	x.closeReader()
	return e.commit(ctx, x, t)
}

// pump moves the bytes. Never wrap both ends in plain Reader/Writer: pkg/sftp only
// pipelines requests through WriteTo and ReadFrom, so the source's WriterTo wins, then the
// destination's ReaderFrom, then a 1 MiB buffer.
func (e *Engine) pump(x *xfer, add func(int64)) error {
	if wt, ok := x.r.(io.WriterTo); ok {
		// WriteTo hands over small chunks; pkg/sftp only pipelines writes larger than one
		// packet, so batch them. Counting sits below the buffer so it reflects bytes written.
		bw := bufio.NewWriterSize(&countWriter{w: x.w, add: add}, writeBatchSize)
		if _, err := wt.WriteTo(bw); err != nil {
			return err
		}
		return bw.Flush()
	}
	if rf, ok := x.w.(io.ReaderFrom); ok {
		_, err := rf.ReadFrom(&countReader{r: x.r, add: add})
		return err
	}
	bp := e.bufs.Get().(*[]byte)
	defer e.bufs.Put(bp)
	_, err := io.CopyBuffer(&countWriter{w: x.w, add: add}, x.r, *bp)
	return err
}

// commit verifies the byte count and publishes the file; anything else aborts the writer.
func (e *Engine) commit(ctx context.Context, x *xfer, t *task) error {
	if got := x.w.Written(); got != t.size {
		x.cancel()
		return fmt.Errorf("copy %s: wrote %d of %d bytes: %w", t.src, got, t.size, vfs.ErrSizeMismatch)
	}
	if !x.beginCommit() {
		return ctx.Err()
	}
	if err := x.w.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", t.dst, err)
	}
	return nil
}

// moveFile moves one file. On a single connection with nothing at the destination it is a
// Rename: instant, no data copied. Otherwise it is a copy, and the source is deleted only
// after the copy has committed, so an error or cancellation leaves the file at the source,
// the destination, or both.
func (e *Engine) moveFile(j *job, t *task) error {
	if t.rename {
		err := j.src.Rename(j.ctx, t.src, t.dst)
		switch {
		case err == nil:
			t.done.Add(t.size)
			j.doneBytes.Add(t.size)
			return nil
		case j.ctx.Err() != nil:
			return j.ctx.Err()
		case errors.Is(err, vfs.ErrExist):
			// Something appeared at dst after the walk looked. Copying would replace it unasked.
			t.noRetry = true
			return fmt.Errorf("rename %s: %w", t.src, err)
		}
		// S3 objects over 5 GiB, a different filesystem and the like: stream it instead.
		// From here on dst may hold our own copy, so a retry must not try to rename again.
		t.rename = false
		e.log.DebugContext(j.ctx, "rename failed, falling back to copy", "job", j.id, "src", t.src, "dst", t.dst, "err", err)
	}
	if err := e.copyFile(j, t); err != nil {
		return err
	}
	return e.removeCopied(j, t)
}

// removeCopied deletes the source of a file that has just been committed at the destination.
// It ignores cancellation: stopping here would leave the file in both places, and Resume
// would then ask about a conflict with itself.
func (e *Engine) removeCopied(j *job, t *task) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(j.ctx), removeCopiedTimeout)
	defer cancel()
	if err := j.src.Remove(ctx, t.src); err != nil && !errors.Is(err, vfs.ErrNotExist) {
		return fmt.Errorf("copied to %s, but removing the source failed: %w", t.dst, err)
	}
	return nil
}

// remove deletes one file, link, or (emptied) directory. Already gone counts as deleted.
func (e *Engine) remove(j *job, t *task) error {
	if err := j.src.Remove(j.ctx, t.src); err != nil && !errors.Is(err, vfs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", t.src, err)
	}
	return nil
}
