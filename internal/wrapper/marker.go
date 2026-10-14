package wrapper

import (
	"bytes"
	"io"
	"os"
)

// wrapperMarker identifies an exe-env-wrapper build from its own bytes. Three
// callers need the answer: next-on-PATH resolution, which must not hand off to
// another wrapper; adapt, which must refuse to adapt a wrapper rather than
// build indirection through one; and upgrade, which must refuse to replace
// anything that is not one.
//
// The literal is referenced below, so every build carries it in its string
// table. The UUID is arbitrary; it only has to be long enough not to appear in
// an unrelated binary by chance.
const wrapperMarker = "exe-env-wrapper marker 4f8c2ad1-6b93-47e5-9c01-3d7ae85f2b64"

// markerScanChunk is the read size for scanning a candidate. Binaries get
// large — claude.exe in a .local/bin directory is well over 200 MB — so the
// scan streams rather than reading the file in.
const markerScanChunk = 64 * 1024

// stampTailLimit is how much of what follows a needle is examined. Long enough
// for any version string, short enough that a hit costs nothing.
const stampTailLimit = 64

// acceptAny takes the first hit, for callers that only ask whether a needle is
// present at all.
func acceptAny(string) bool { return true }

// scanFileFor finds needle in a file and returns the bytes following the first
// hit that accept approves.
//
// A file that cannot be read reports false. The callers are deciding whether
// to skip, refuse or replace a candidate, and an unreadable file fails later
// on its own terms with a better message than this scan could give.
func scanFileFor(path, needle string, accept func(string) bool) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	return scanReaderFor(f, needle, markerScanChunk, accept)
}

// scanReaderFor finds each occurrence of needle in r and returns the bytes
// following the first one accept approves.
//
// Hits are filtered rather than taken as the first match, because a needle
// built from a constant in this package also exists in the binary as that
// constant — ahead of whatever stamped value is actually being looked for.
//
// The read is streamed, because the binaries involved get large. Enough of
// each buffer is kept to reassemble a needle landing across a chunk boundary,
// and a hit whose tail is not yet complete is left for the next read rather
// than judged on truncated evidence.
//
// chunk is a parameter so the boundary cases can be tested without a 64 KB
// fixture.
func scanReaderFor(r io.Reader, needle string, chunk int, accept func(string) bool) (string, bool) {
	want := []byte(needle)
	overlap := len(want) + stampTailLimit
	if chunk < overlap {
		chunk = overlap
	}

	buf := make([]byte, 0, chunk+overlap)
	tmp := make([]byte, chunk)

	// examine judges every complete hit in buf. final says no more data is
	// coming, so a short tail is all there will ever be and has to be judged
	// rather than deferred.
	examine := func(final bool) (string, bool) {
		for from := 0; ; {
			at := bytes.Index(buf[from:], want)
			if at < 0 {
				return "", false
			}
			at += from
			tail := buf[at+len(want):]
			if len(tail) < stampTailLimit && !final {
				// The tail may continue into the next read; judging it now
				// could reject a hit on incomplete evidence.
				return "", false
			}
			if len(tail) > stampTailLimit {
				tail = tail[:stampTailLimit]
			}
			if accept(string(tail)) {
				return string(tail), true
			}
			from = at + len(want)
		}
	}

	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if tail, ok := examine(false); ok {
				return tail, true
			}
			if len(buf) > overlap {
				buf = append(buf[:0], buf[len(buf)-overlap:]...)
			}
		}
		if readErr != nil {
			// The last read carried no data, or failed. Whatever is still in
			// the buffer is final, including a hit held back for a tail that
			// never arrived.
			return examine(true)
		}
	}
}

// readerContainsMarker reports whether the marker appears anywhere in r.
func readerContainsMarker(r io.Reader, chunk int) (bool, error) {
	_, ok := scanReaderFor(r, wrapperMarker, chunk, acceptAny)
	return ok, nil
}

// isWrapperBinary reports whether path is an exe-env-wrapper build.
func isWrapperBinary(path string) bool {
	_, ok := scanFileFor(path, wrapperMarker, acceptAny)
	return ok
}
