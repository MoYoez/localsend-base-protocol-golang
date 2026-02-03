package tool

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// NextAvailablePath returns the first path under dir that does not exist, using fileName
// and if it exists, trying base-2.ext, base-3.ext, ... (e.g. txt.txt -> txt-2.txt, txt-3.txt).
func NextAvailablePath(dir, fileName string) string {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(filepath.Base(fileName), ext)
	if base == "" {
		base = fileName
		ext = ""
	}
	try := filepath.Join(dir, fileName)
	if _, err := os.Stat(try); os.IsNotExist(err) {
		return try
	}
	for n := 2; ; n++ {
		try = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, n, ext))
		if _, err := os.Stat(try); os.IsNotExist(err) {
			return try
		}
	}
}

// CopyWithContext copies from src to dst while respecting context cancellation.
func CopyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 2*1024*1024) // 2MB buffer
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if writeErr == nil {
					writeErr = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
			if nr != nw {
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
