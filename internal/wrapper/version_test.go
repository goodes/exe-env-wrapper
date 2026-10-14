package wrapper

import (
	"path/filepath"
	"strings"
	"testing"
)

// The version has to be readable out of a compiled wrapper without running
// it, because an installed wrapper is named after the tool it stands in for:
// asking it for its version would pass "version" to that tool instead. A bare
// ldflags value is in the binary but unfindable, so it is stamped with a
// prefix to search for.
func TestReadVersionFromBuiltBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, executableName("exe-env-wrapper"))
	goBuildVersioned(t, repoRoot(t), "./cmd/exe-env-wrapper", path, "v9.9.9-probe")

	got, ok := readVersionFromFile(path)
	if !ok {
		t.Fatalf("no version stamp found in %s", path)
	}
	if got != "v9.9.9-probe" {
		t.Errorf("version = %q, want %q", got, "v9.9.9-probe")
	}
}

// A binary that is not a wrapper has no stamp, and must not be mistaken for
// one carrying an odd version.
func TestReadVersionFromFileRejectsOtherBinaries(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, executableName("not-a-wrapper"))
	buildChildAs(t, path)

	if version, ok := readVersionFromFile(path); ok {
		t.Errorf("found version %q in a binary that is not a wrapper", version)
	}
}

// An unstamped wrapper is the state of everything installed before the stamp
// existed. It has to read as "a wrapper, version unknown" rather than as not
// a wrapper, or upgrade could never replace one.
func TestReadVersionFromFileHandlesAnUnstampedWrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, executableName("old-wrapper"))
	// The marker without any stamp, which is what v0.3.2 and earlier carry.
	writeFile(t, path, "padding "+wrapperMarker+" more padding")

	if !isWrapperBinary(path) {
		t.Fatal("an unstamped wrapper is no longer recognised as a wrapper")
	}
	if version, ok := readVersionFromFile(path); ok {
		t.Errorf("found version %q where none was stamped", version)
	}
}

// The running binary's own stamp and its reported version are the same thing,
// so they cannot drift apart.
func TestVersionMatchesItsOwnStamp(t *testing.T) {
	if !strings.HasPrefix(VersionStamp, versionStampPrefix) {
		t.Fatalf("VersionStamp = %q, want it to start with %q", VersionStamp, versionStampPrefix)
	}
	if want := strings.TrimPrefix(VersionStamp, versionStampPrefix); Version != want {
		t.Errorf("Version = %q, want %q from the stamp", Version, want)
	}
}
