package wrapper

import (
	"path/filepath"
	"testing"
)

// upgrade replaces an installed wrapper with this binary, so it has to know
// which of the two builds it is replacing. Swapping a windowless wrapper for a
// console one would reintroduce the window flash under a host that has no
// console, silently.
func TestReadVariantFromBuiltBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, executableName("exe-env-wrapper"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", path)

	got, ok := readVariantFromFile(path)
	if !ok {
		t.Fatalf("no variant stamp found in %s", path)
	}
	if got != variantConsole {
		t.Errorf("variant = %q, want %q", got, variantConsole)
	}
}

// Reading the variant makes no claim about the file being a wrapper: every
// Windows binary has a subsystem. Whether something is a wrapper is
// isWrapperBinary's question, and upgrade asks it first — which is what stops
// it replacing the wrong file.
func TestReadVariantSaysNothingAboutBeingAWrapper(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, executableName("not-a-wrapper"))
	buildChildAs(t, path)

	if isWrapperBinary(path) {
		t.Error("an ordinary binary was taken for a wrapper")
	}
	// It still has a subsystem, and that is fine.
	if _, ok := readVariantFromFile(path); !ok {
		t.Error("an ordinary Windows binary should still report a subsystem")
	}
}

// Something that is not a PE file at all has no subsystem to report.
func TestReadVariantOnANonExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	writeFile(t, path, "not an executable at all")

	_, ok := readVariantFromFile(path)
	if executableName("x") == "x.exe" && ok {
		t.Error("a text file reported a subsystem")
	}
}

// A build that has commands is the console build: the windowless one is a
// runtime payload with no commands at all.
func TestBuildVariantMatchesTheBuildTags(t *testing.T) {
	want := variantWindowless
	if ownFlagsEnabled {
		want = variantConsole
	}
	if got := buildVariant(); got != want {
		t.Errorf("buildVariant() = %q, want %q", got, want)
	}
}

// A real windowless build has to be distinguishable from a real console one.
// This is the assertion that matters: the previous approach passed its tests
// against hand-written stamps while both real builds read as console.
func TestRealBuildsAreDistinguishable(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	if executableName("x") != "x.exe" {
		t.Skip("the two builds differ only on Windows")
	}

	dir := t.TempDir()
	console := filepath.Join(dir, executableName("console"))
	windowless := filepath.Join(dir, executableName("windowless"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", console)
	goBuildWindowless(t, windowless, "v0.0.0-test")

	if got, ok := readVariantFromFile(console); !ok || got != variantConsole {
		t.Errorf("the console build reads as %q (known=%v), want %q", got, ok, variantConsole)
	}
	if got, ok := readVariantFromFile(windowless); !ok || got != variantWindowless {
		t.Errorf("the windowless build reads as %q (known=%v), want %q", got, ok, variantWindowless)
	}
}
