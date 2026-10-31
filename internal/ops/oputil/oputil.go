// Package oputil holds small I/O helpers shared by operation implementations.
package oputil

import (
	"context"
	"io"
)

// Copy copies src to dst, reporting progress through report. It returns the
// number of bytes written.
func Copy(ctx context.Context, dst io.Writer, src io.Reader, report func(done int64)) (int64, error) {
	buf := make([]byte, 1<<20)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			nw, writeErr := dst.Write(buf[:n])
			written += int64(nw)
			if report != nil {
				report(written)
			}
			if writeErr != nil {
				return written, writeErr
			}
			if nw != n {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			return written, readErr
		}
	}
}
