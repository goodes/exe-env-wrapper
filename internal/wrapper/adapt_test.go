package wrapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type adaptFixture struct {
	dir       string
	toolPath  string
	sourceExe string
	out       strings.Builder
}

func newAdaptFixture(t *testing.T, toolName, body string) *adaptFixture {
	t.Helper()
	root := t.TempDir()
	f := &adaptFixture{
		dir:       root,
		toolPath:  filepath.Join(root, executableName(toolName)),
		sourceExe: filepath.Join(root, "source", executableName("exe-env-wrapper")),
	}
	writeFile(t, f.toolPath, body)
	writeFile(t, f.sourceExe, "the wrapper binary, carrying "+wrapperMarker)
	return f
}

func (f *adaptFixture) params() adaptParams {
	return adaptParams{
		file:      f.toolPath,
		ownDir:    f.dir,
		sourceExe: f.sourceExe,
		out:       &f.out,
	}
}

// The original is renamed aside with a tag of its contents, and the wrapper
// takes its place under the name callers already use.
func TestAdaptRenamesAsideAndStandsIn(t *testing.T) {
	f := newAdaptFixture(t, "yt-dlp", "the real tool")

	if err := adaptBinary(f.params()); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}

	if !isWrapperBinary(f.toolPath) {
		t.Errorf("%s is not the wrapper after adapting", f.toolPath)
	}

	aside := findAside(t, f.dir, "yt-dlp")
	body, err := os.ReadFile(aside)
	if err != nil {
		t.Fatalf("reading the moved binary: %v", err)
	}
	if string(body) != "the real tool" {
		t.Errorf("the moved binary = %q, want the original contents", body)
	}

	config, err := os.ReadFile(filepath.Join(f.dir, "yt-dlp.yaml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if want := "target: " + filepath.Base(aside); !strings.Contains(string(config), want) {
		t.Errorf("config = %q, want %q", config, want)
	}
	if !strings.Contains(string(config), "${system.no_proxy}") {
		t.Errorf("config = %q, want the system proxy defaults", config)
	}
}

// findAside returns the single hashed sibling of a tool.
func findAside(t *testing.T, dir, stem string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, stem+"-*"))
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, m := range matches {
		if !strings.HasSuffix(m, ".yaml") {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one hashed binary, got %v", found)
	}
	return found[0]
}

// The hash is derived from the contents, so the name is stable for the same
// binary and different for a different one.
func TestAdaptHashTracksContents(t *testing.T) {
	a := newAdaptFixture(t, "yt-dlp", "version one")
	if err := adaptBinary(a.params()); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}
	first := filepath.Base(findAside(t, a.dir, "yt-dlp"))

	b := newAdaptFixture(t, "yt-dlp", "version one")
	if err := adaptBinary(b.params()); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}
	if got := filepath.Base(findAside(t, b.dir, "yt-dlp")); got != first {
		t.Errorf("identical contents gave %q and %q, want the same name", first, got)
	}

	c := newAdaptFixture(t, "yt-dlp", "version two")
	if err := adaptBinary(c.params()); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}
	if got := filepath.Base(findAside(t, c.dir, "yt-dlp")); got == first {
		t.Errorf("different contents gave the same name %q", got)
	}
}

// Re-running must not rename the wrapper aside and point the config at it.
func TestAdaptRefusesAnAlreadyAdaptedBinary(t *testing.T) {
	f := newAdaptFixture(t, "yt-dlp", "the real tool")
	if err := adaptBinary(f.params()); err != nil {
		t.Fatalf("first adaptBinary: %v", err)
	}

	err := adaptBinary(f.params())
	if err == nil || !strings.Contains(err.Error(), "already an exe-env-wrapper") {
		t.Fatalf("err = %v, want a refusal naming the cause", err)
	}
}

// An existing config is somebody's edited file; --force replaces it.
func TestAdaptLeavesAnExistingConfigAloneWithoutForce(t *testing.T) {
	f := newAdaptFixture(t, "yt-dlp", "the real tool")
	writeFile(t, filepath.Join(f.dir, "yt-dlp.yaml"), "env:\n  MINE: yes\n")

	if err := adaptBinary(f.params()); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(f.dir, "yt-dlp.yaml"))
	if !strings.Contains(string(body), "MINE: yes") {
		t.Errorf("config = %q, want the hand-edited file preserved", body)
	}
	if !strings.Contains(f.out.String(), "--force") {
		t.Errorf("output = %q, want it to mention how to replace the config", f.out.String())
	}
}

func TestAdaptForceReplacesConfig(t *testing.T) {
	f := newAdaptFixture(t, "yt-dlp", "the real tool")
	writeFile(t, filepath.Join(f.dir, "yt-dlp.yaml"), "env:\n  MINE: yes\n")

	p := f.params()
	p.force = true
	if err := adaptBinary(p); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(f.dir, "yt-dlp.yaml"))
	if strings.Contains(string(body), "MINE: yes") {
		t.Errorf("config = %q, want it replaced under --force", body)
	}
	if !strings.Contains(string(body), "target: ") {
		t.Errorf("config = %q, want the target recorded", body)
	}
}

// A bare name is taken from the wrapper's own directory, never from PATH: a
// PATH lookup would find the wrong file to replace.
func TestAdaptBareNameUsesOwnDirectory(t *testing.T) {
	f := newAdaptFixture(t, "yt-dlp", "the real tool")
	p := f.params()
	p.file = "yt-dlp" + exeSuffix()

	if err := adaptBinary(p); err != nil {
		t.Fatalf("adaptBinary: %v", err)
	}
	if !isWrapperBinary(f.toolPath) {
		t.Error("the bare name did not resolve to the file in the wrapper's own directory")
	}
}

func TestAdaptRefusesAMissingFile(t *testing.T) {
	f := newAdaptFixture(t, "yt-dlp", "the real tool")
	p := f.params()
	p.file = filepath.Join(f.dir, executableName("absent"))

	err := adaptBinary(p)
	if err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("err = %v, want a not-a-file error", err)
	}
}
