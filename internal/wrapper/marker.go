package wrapper

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// wrapperMarker identifies an exe-env-wrapper build from its own bytes. Two
// callers need the answer: next-on-PATH resolution, which must not hand off to
// another wrapper, and --adapt, which must refuse to adapt a wrapper rather
// than build indirection through one.
//
// The literal is referenced below, so every build carries it in its string
// table. The UUID is arbitrary; it only has to be long enough not to appear in
// an unrelated binary by chance.
const wrapperMarker = "exe-env-wrapper marker 4f8c2ad1-6b93-47e5-9c01-3d7ae85f2b64"

// markerScanChunk is the read size for scanning a candidate. Binaries get
// large — claude.exe in a .local/bin directory is well over 200 MB — so the
// scan streams rather than reading the file in.
const markerScanChunk = 64 * 1024

// readerContainsMarker reports whether the marker appears anywhere in r.
//
// Each read keeps the tail of the previous buffer, so a marker landing across
// a chunk boundary is still found. chunk is a parameter so the boundary case
// can be tested without a 64 KB fixture.
func readerContainsMarker(r io.Reader, chunk int) (bool, error) {
	marker := []byte(wrapperMarker)
	// The window has to hold a whole marker plus one fresh byte, or a marker
	// split across reads could never be assembled.
	if chunk < len(marker)+1 {
		chunk = len(marker) + 1
	}
	buf := make([]byte, 0, chunk+len(marker))
	tmp := make([]byte, chunk)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if bytes.Contains(buf, marker) {
				return true, nil
			}
			// Keep only what a marker could still straddle into the next read.
			if overlap := len(marker) - 1; len(buf) > overlap {
				buf = append(buf[:0], buf[len(buf)-overlap:]...)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
	}
}

// isWrapperBinary reports whether path is an exe-env-wrapper build. A file
// that cannot be read is reported as not a wrapper: the callers are deciding
// whether to skip or refuse a candidate, and an unreadable file fails later on
// its own terms, with a better message than this scan could give.
func isWrapperBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	found, err := readerContainsMarker(f, markerScanChunk)
	return err == nil && found
}
