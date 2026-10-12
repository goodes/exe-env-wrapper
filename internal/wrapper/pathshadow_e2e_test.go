package wrapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildChildAs builds the stand-in tool to an arbitrary path, so a test can
// place it somewhere other than beside the wrapper.
func buildChildAs(t *testing.T, path string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(childSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, src, ".", path)
}

// A config with no target resolves the next match for the wrapper's own name
// on PATH. This is the whole PATH-shadowing mechanism: the wrapper sits under
// the tool's name in a directory earlier on PATH, and finds the real tool
// without being told where it is.
func TestEndToEndResolvesNextOnPathWhenTargetAbsent(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	wrapperDir := t.TempDir()
	realDir := t.TempDir()

	buildChildAs(t, filepath.Join(realDir, executableName("yt")))

	wrapperPath := filepath.Join(wrapperDir, executableName("yt"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	write(t, filepath.Join(wrapperDir, "yt.yaml"), "env:\n  INHERITED: from-wrapper\n")

	path := wrapperDir + string(os.PathListSeparator) + realDir
	res := run(t, wrapperPath, "", []string{"PATH=" + path}, "an-arg")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ARG:an-arg") {
		t.Errorf("stdout = %q, want the argument to have reached the real tool", res.stdout)
	}
	if !strings.Contains(res.stdout, "ENV:INHERITED=from-wrapper") {
		t.Errorf("stdout = %q, want the wrapper's environment applied", res.stdout)
	}
}

// A wrapper that is first on PATH must not resolve to itself, however its own
// directory is spelled there.
func TestEndToEndDoesNotResolveToItself(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, executableName("yt"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	write(t, filepath.Join(wrapperDir, "yt.yaml"), "env:\n  INHERITED: x\n")

	// The wrapper's own directory, twice, under two spellings, and nothing else.
	path := strings.Join([]string{
		wrapperDir,
		filepath.Join(wrapperDir, "."),
	}, string(os.PathListSeparator))
	res := run(t, wrapperPath, "", []string{"PATH=" + path})

	if res.code != ExitError {
		t.Fatalf("exit code = %d, want %d (it should refuse, not loop)\nstdout: %s",
			res.code, ExitError, res.stdout)
	}
	if !strings.Contains(res.stderr, "yt") {
		t.Errorf("stderr = %q, want a message naming what it searched for", res.stderr)
	}
}

// With no config file at all, the wrapper applies built-in defaults and still
// resolves its target from PATH. This is what makes installing a wrapper one
// step: copy the binary in under the tool's name, nothing else.
func TestEndToEndNoConfigUsesDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	wrapperDir := t.TempDir()
	realDir := t.TempDir()
	buildChildAs(t, filepath.Join(realDir, executableName("yt")))

	wrapperPath := filepath.Join(wrapperDir, executableName("yt"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	// Deliberately no yt.yaml.

	path := wrapperDir + string(os.PathListSeparator) + realDir
	res := run(t, wrapperPath, "", []string{"PATH=" + path}, "an-arg")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ARG:an-arg") {
		t.Errorf("stdout = %q, want the argument to have reached the real tool", res.stdout)
	}
}

// The defaults must do no harm. On a machine with no system proxy configured,
// ${system.http_proxy} expands to nothing, and setting HTTP_PROXY to that
// would blank out a proxy the caller had deliberately set. A default that
// expands to nothing leaves the inherited value alone instead.
func TestEndToEndDefaultsDoNotBlankInheritedProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	wrapperDir := t.TempDir()
	realDir := t.TempDir()
	buildChildAs(t, filepath.Join(realDir, executableName("yt")))

	wrapperPath := filepath.Join(wrapperDir, executableName("yt"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)

	path := wrapperDir + string(os.PathListSeparator) + realDir
	res := run(t, wrapperPath, "", []string{
		"PATH=" + path,
		"HTTP_PROXY=http://caller.example:8080",
	})

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ENV:HTTP_PROXY=http://caller.example:8080") {
		t.Errorf("stdout = %q, want the caller's HTTP_PROXY left intact", res.stdout)
	}
}
