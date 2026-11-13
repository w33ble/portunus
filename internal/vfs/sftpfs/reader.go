package sftpfs

import (
	"context"
	"io"
	"os"
	"sync/atomic"

	"github.com/pkg/sftp"
)

// reader exposes sftp.File's pipelined WriteTo. sftp.File.Close blocks while WriteTo
// runs, so Close first flips a flag that makes the destination writer fail, which ends
// WriteTo at the next chunk.
type reader struct {
	f      *sftp.File
	ctx    context.Context
	closed atomic.Bool
}

func (r *reader) check() error {
	if r.closed.Load() {
		return os.ErrClosed
	}
	return r.ctx.Err()
}

func (r *reader) Read(b []byte) (int, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	return r.f.Read(b)
}

func (r *reader) WriteTo(w io.Writer) (int64, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	return r.f.WriteTo(guardWriter{w: w, r: r})
}

func (r *reader) Close() error {
	if r.closed.Swap(true) {
		return nil
	}
	return r.f.Close()
}

type guardWriter struct {
	w io.Writer
	r *reader
}

func (g guardWriter) Write(b []byte) (int, error) {
	if err := g.r.check(); err != nil {
		return 0, err
	}
	return g.w.Write(b)
}
