package s3fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/w33ble/portunus/internal/vfs"
)

const (
	minPartSize  = 5 << 20
	maxPartSize  = 5 << 30
	maxParts     = 10000
	autoPartSize = 8 << 20
	// bufferBudget caps the memory one writer may hold across its part buffers.
	bufferBudget = 256 << 20
	// maxParallel is the number of parts uploaded concurrently per writer.
	maxParallel  = 4
	abortTimeout = 30 * time.Second
)

// partSizeFor picks a part size that keeps size within the 10,000 part limit.
func partSizeFor(size, configured int64) int64 {
	ps := configured
	if ps <= 0 {
		ps = autoPartSize
	}
	ps = max(ps, minPartSize)
	if size > 0 {
		need := (size + maxParts - 1) / maxParts
		need = (need + (1 << 20) - 1) &^ ((1 << 20) - 1) // round up to MiB
		ps = max(ps, need)
	}
	return min(ps, maxPartSize)
}

// bufferCount is how many part buffers (in flight plus filling) fit the memory budget.
func bufferCount(partSize int64) int {
	return int(max(1, min(maxParallel+1, bufferBudget/partSize))) //nolint:gosec // bounded by maxParallel+1
}

func (f *FS) Create(ctx context.Context, p string, opts vfs.CreateOpts) (vfs.Writer, error) {
	l, err := f.parse(p)
	if err != nil {
		return nil, err
	}
	if l.key == "" {
		return nil, fmt.Errorf("s3 create %s: not an object path", p)
	}
	ps := partSizeFor(opts.Size, f.cfg.PartSize)
	initCap := ps
	if opts.Size > 0 && opts.Size < ps {
		initCap = opts.Size + 1
	}
	wctx, cancel := context.WithCancel(ctx)
	n := bufferCount(ps)
	return &writer{
		parent:   ctx,
		ctx:      wctx,
		cancel:   cancel,
		client:   f.clientFor(ctx, l.bucket),
		loc:      l,
		opts:     opts,
		partSize: int(ps),      //nolint:gosec // <= 5 GiB
		initCap:  int(initCap), //nolint:gosec // <= partSize+1
		maxBufs:  n,
		free:     make(chan []byte, n),
	}, nil
}

type writer struct {
	parent context.Context
	ctx    context.Context
	cancel context.CancelFunc
	client *s3.Client
	loc    loc
	opts   vfs.CreateOpts

	partSize int
	initCap  int
	maxBufs  int
	free     chan []byte
	alloc    int

	cur      []byte
	written  int64
	uploadID *string
	nextPart int32
	finished bool

	wg    sync.WaitGroup
	mu    sync.Mutex
	parts []types.CompletedPart
	err   error
}

func (w *writer) Written() int64 { return w.written }

func (w *writer) fail(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.mu.Unlock()
	w.cancel()
}

func (w *writer) failure() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

// take returns an empty buffer, blocking while all buffers are in flight.
func (w *writer) take() error {
	select {
	case b := <-w.free:
		w.cur = b[:0]
		return nil
	default:
	}
	if w.alloc < w.maxBufs {
		w.alloc++
		w.cur = make([]byte, 0, w.initCap)
		return nil
	}
	select {
	case b := <-w.free:
		w.cur = b[:0]
		return nil
	case <-w.ctx.Done():
		if err := w.failure(); err != nil {
			return err
		}
		return w.ctx.Err()
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.finished {
		return 0, errors.New("s3 write: writer already finished")
	}
	total := 0
	for len(p) > 0 {
		if err := w.failure(); err != nil {
			return total, err
		}
		if w.cur == nil {
			if err := w.take(); err != nil {
				return total, err
			}
		}
		n := min(len(p), w.partSize-len(w.cur))
		w.cur = append(w.cur, p[:n]...)
		p = p[n:]
		total += n
		w.written += int64(n)
		if len(w.cur) >= w.partSize {
			if err := w.flush(); err != nil {
				return total, err
			}
		}
	}
	return total, nil
}

// ReadFrom reads straight into part buffers, skipping the intermediate io.Copy buffer.
func (w *writer) ReadFrom(r io.Reader) (int64, error) {
	if w.finished {
		return 0, errors.New("s3 write: writer already finished")
	}
	var total int64
	for {
		if err := w.failure(); err != nil {
			return total, err
		}
		if w.cur == nil {
			if err := w.take(); err != nil {
				return total, err
			}
		}
		if len(w.cur) == cap(w.cur) {
			w.cur = slices.Grow(w.cur, 32<<10)
		}
		n, err := r.Read(w.cur[len(w.cur):min(cap(w.cur), w.partSize)])
		w.cur = w.cur[:len(w.cur)+n]
		w.written += int64(n)
		total += int64(n)
		if len(w.cur) >= w.partSize {
			if ferr := w.flush(); ferr != nil {
				return total, ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

func (w *writer) startUpload() error {
	out, err := w.client.CreateMultipartUpload(w.ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(w.loc.bucket),
		Key:    aws.String(w.loc.key),
	})
	if err != nil {
		err = mapErr("create", w.loc.path(), err)
		w.fail(err)
		return err
	}
	w.uploadID = out.UploadId
	return nil
}

// flush hands the current buffer to an uploader goroutine.
func (w *writer) flush() error {
	if w.uploadID == nil {
		if err := w.startUpload(); err != nil {
			return err
		}
	}
	if w.nextPart >= maxParts {
		err := fmt.Errorf("s3 write %s: more than %d parts; declared size too small", w.loc.path(), maxParts)
		w.fail(err)
		return err
	}
	w.nextPart++
	buf := w.cur
	w.cur = nil
	w.wg.Add(1)
	go w.uploadPart(w.nextPart, buf)
	return nil
}

func (w *writer) uploadPart(num int32, buf []byte) {
	defer w.wg.Done()
	defer func() { w.free <- buf }()
	out, err := w.client.UploadPart(w.ctx, &s3.UploadPartInput{
		Bucket:        aws.String(w.loc.bucket),
		Key:           aws.String(w.loc.key),
		UploadId:      w.uploadID,
		PartNumber:    aws.Int32(num),
		Body:          bytes.NewReader(buf),
		ContentLength: aws.Int64(int64(len(buf))),
	})
	if err != nil {
		w.fail(mapErr(fmt.Sprintf("upload part %d of", num), w.loc.path(), err))
		return
	}
	w.mu.Lock()
	w.parts = append(w.parts, types.CompletedPart{
		PartNumber:        aws.Int32(num),
		ETag:              out.ETag,
		ChecksumCRC32:     out.ChecksumCRC32,
		ChecksumCRC32C:    out.ChecksumCRC32C,
		ChecksumCRC64NVME: out.ChecksumCRC64NVME,
		ChecksumSHA1:      out.ChecksumSHA1,
		ChecksumSHA256:    out.ChecksumSHA256,
	})
	w.mu.Unlock()
}

func (w *writer) Commit() error {
	if w.finished {
		return errors.New("s3 commit: writer already finished")
	}
	if err := w.commit(); err != nil {
		_ = w.Abort()
		return err
	}
	w.finished = true
	w.cancel()
	return nil
}

func (w *writer) commit() error {
	if w.written != w.opts.Size {
		return fmt.Errorf("s3 commit %s: wrote %d bytes, expected %d: %w", w.loc.path(), w.written, w.opts.Size, vfs.ErrSizeMismatch)
	}
	if err := w.failure(); err != nil {
		return err
	}
	if w.uploadID == nil {
		_, err := w.client.PutObject(w.ctx, &s3.PutObjectInput{
			Bucket:        aws.String(w.loc.bucket),
			Key:           aws.String(w.loc.key),
			Body:          bytes.NewReader(w.cur),
			ContentLength: aws.Int64(int64(len(w.cur))),
		})
		return mapErr("put", w.loc.path(), err)
	}
	if len(w.cur) > 0 {
		if err := w.flush(); err != nil {
			return err
		}
	}
	w.wg.Wait()
	if err := w.failure(); err != nil {
		return err
	}
	sort.Slice(w.parts, func(i, j int) bool { return *w.parts[i].PartNumber < *w.parts[j].PartNumber })
	_, err := w.client.CompleteMultipartUpload(w.ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(w.loc.bucket),
		Key:             aws.String(w.loc.key),
		UploadId:        w.uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: w.parts},
	})
	return mapErr("complete", w.loc.path(), err)
}

// Abort stops uploads and discards the multipart upload. Safe to call after a failed Commit.
func (w *writer) Abort() error {
	if w.finished {
		return nil
	}
	w.finished = true
	w.cancel()
	w.wg.Wait()
	if w.uploadID == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(w.parent), abortTimeout)
	defer cancel()
	_, err := w.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.loc.bucket),
		Key:      aws.String(w.loc.key),
		UploadId: w.uploadID,
	})
	return mapErr("abort", w.loc.path(), err)
}
