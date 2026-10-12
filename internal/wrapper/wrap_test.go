package wrapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile makes a file with the given contents, creating parents.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// wrapFixture lays out a wrapper folder, a tool directory and a stand-in
// wrapper binary.
type wrapFixture struct {
	wrapperDir string
	toolDir    string
	sourceExe  string
	toolPath   string
	out        strings.Builder
}

func newWrapFixture(t *testing.T, toolName string) *wrapFixture {
	t.Helper()
	root := t.TempDir()
	f := &wrapFixture{
		wrapperDir: filepath.Join(root, "wrapper"),
		toolDir:    filepath.Join(root, "bin"),
		sourceExe:  filepath.Join(root, "source", executableName("exe-env-wrapper")),
	}
	f.toolPath = filepath.Join(f.toolDir, executableName(toolName))
	writeFile(t, f.toolPath, "the real tool")
	writeFile(t, f.sourceExe, "the wrapper binary, carrying "+wrapperMarker)
	return f
}

func (f *wrapFixture) params(tool string, pathEntries []string) wrapParams {
	return wrapParams{
		tool:        tool,
		wrapperDir:  f.wrapperDir,
		sourceExe:   f.sourceExe,
		pathEntries: pathEntries,
		probe:       realPathProbe(),
		out:         &f.out,
	}
}

// A bare name resolves on PATH and installs without a config file: resolution
// stays dynamic, so upgrading or moving the tool does not break the wrapper.
func TestWrapByBareNameNeedsNoConfig(t *testing.T) {
	f := newWrapFixture(t, "yt-dlp")
	p := f.params("yt-dlp", []string{f.wrapperDir, f.toolDir})

	if err := wrapTool(p); err != nil {
		t.Fatalf("wrapTool: %v", err)
	}

	installed := filepath.Join(f.wrapperDir, executableName("yt-dlp"))
	if !isRegularFile(installed) {
		t.Fatalf("no wrapper installed at %s", installed)
	}
	if !isWrapperBinary(installed) {
		t.Error("the installed file is not the wrapper binary")
	}
	if _, err := os.Stat(filepath.Join(f.wrapperDir, "yt-dlp.yaml")); !os.IsNotExist(err) {
		t.Error("a config file was written for a bare name, which needs none")
	}
	// The original must be untouched: that is the difference from --adapt.
	if body, _ := os.ReadFile(f.toolPath); string(body) != "the real tool" {
		t.Error("the original binary was modified")
	}
}

// A path pins exactly that file, which has to be recorded because it cannot be
// rediscovered.
func TestWrapByPathPinsTarget(t *testing.T) {
	f := newWrapFixture(t, "yt-dlp")
	p := f.params(f.toolPath, []string{f.wrapperDir, f.toolDir})

	if err := wrapTool(p); err != nil {
		t.Fatalf("wrapTool: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(f.wrapperDir, "yt-dlp.yaml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !strings.Contains(string(body), "target: "+f.toolPath) {
		t.Errorf("config = %q, want the pinned target", body)
	}
	if !strings.Contains(string(body), "${system.no_proxy}") {
		t.Errorf("config = %q, want the system proxy defaults", body)
	}
}

// --config writes the defaults out for editing even when none is needed.
func TestWrapWithConfigFlagWritesDefaults(t *testing.T) {
	f := newWrapFixture(t, "yt-dlp")
	p := f.params("yt-dlp", []string{f.wrapperDir, f.toolDir})
	p.writeConfig = true

	if err := wrapTool(p); err != nil {
		t.Fatalf("wrapTool: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(f.wrapperDir, "yt-dlp.yaml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if strings.Contains(string(body), "target:") {
		t.Errorf("config = %q, want no target so resolution stays dynamic", body)
	}
	if !strings.Contains(string(body), "${system.no_proxy}") {
		t.Errorf("config = %q, want the system proxy defaults", body)
	}
}

// An existing config is somebody's edited file. It is reported, never
// overwritten.
func TestWrapLeavesAnExistingConfigAlone(t *testing.T) {
	f := newWrapFixture(t, "yt-dlp")
	writeFile(t, filepath.Join(f.wrapperDir, "yt-dlp.yaml"), "env:\n  MINE: yes\n")

	p := f.params("yt-dlp", []string{f.wrapperDir, f.toolDir})
	p.writeConfig = true
	if err := wrapTool(p); err != nil {
		t.Fatalf("wrapTool: %v", err)
	}

	body, _ := os.ReadFile(filepath.Join(f.wrapperDir, "yt-dlp.yaml"))
	if !strings.Contains(string(body), "MINE: yes") {
		t.Errorf("config = %q, want the hand-edited file preserved", body)
	}
	if !strings.Contains(f.out.String(), "left alone") {
		t.Errorf("output = %q, want it to say the config was kept", f.out.String())
	}
}

func TestWrapRefusals(t *testing.T) {
	t.Run("refuses another wrapper", func(t *testing.T) {
		f := newWrapFixture(t, "yt-dlp")
		writeFile(t, f.toolPath, "already a wrapper: "+wrapperMarker)

		err := wrapTool(f.params(f.toolPath, []string{f.wrapperDir, f.toolDir}))
		if err == nil || !strings.Contains(err.Error(), "already an exe-env-wrapper") {
			t.Fatalf("err = %v, want a refusal naming the cause", err)
		}
	})

	t.Run("refuses a missing file", func(t *testing.T) {
		f := newWrapFixture(t, "yt-dlp")
		missing := filepath.Join(f.toolDir, executableName("absent"))

		err := wrapTool(f.params(missing, []string{f.wrapperDir, f.toolDir}))
		if err == nil || !strings.Contains(err.Error(), "not a file") {
			t.Fatalf("err = %v, want a not-a-file error", err)
		}
	})

	t.Run("reports a name that is nowhere on PATH", func(t *testing.T) {
		f := newWrapFixture(t, "yt-dlp")

		err := wrapTool(f.params("nothing-here", []string{f.wrapperDir, f.toolDir}))
		if err == nil || !strings.Contains(err.Error(), "cannot wrap nothing-here") {
			t.Fatalf("err = %v, want a cannot-wrap error", err)
		}
	})
}

// A wrapper behind the real tool on PATH is never invoked, so it silently does
// nothing. That has to be said at install time, which is the only moment
// anyone is looking.
func TestWrapWarnsWhenItWillNeverBeReached(t *testing.T) {
	f := newWrapFixture(t, "yt-dlp")
	// The real tool's directory comes first.
	p := f.params("yt-dlp", []string{f.toolDir, f.wrapperDir})

	if err := wrapTool(p); err != nil {
		t.Fatalf("wrapTool: %v", err)
	}

	got := f.out.String()
	if !strings.Contains(got, "WARNING") || !strings.Contains(got, "never be reached") {
		t.Errorf("output = %q, want a warning that the wrapper is unreachable", got)
	}
}

func TestWrapWarnsWhenFolderIsNotOnPath(t *testing.T) {
	f := newWrapFixture(t, "yt-dlp")
	p := f.params("yt-dlp", []string{f.toolDir})

	if err := wrapTool(p); err != nil {
		t.Fatalf("wrapTool: %v", err)
	}
	if !strings.Contains(f.out.String(), "not on PATH") {
		t.Errorf("output = %q, want a warning that the folder is not on PATH", f.out.String())
	}
}

// The windowless build is what a background agent needs, so a sibling one is
// preferred over self-copying. --console overrides it.
func TestWrapperSourceExePrefersWindowlessSibling(t *testing.T) {
	if executableName("x") != "x.exe" {
		t.Skip("the windowless build is a Windows notion")
	}
	dir := t.TempDir()
	console := filepath.Join(dir, "exe-env-wrapper-windows-amd64.exe")
	windowless := filepath.Join(dir, "exe-env-wrapper-w-windows-amd64.exe")
	writeFile(t, console, "console")
	writeFile(t, windowless, "windowless")

	got, isWindowless := wrapperSourceExe(console, false)
	if got != windowless || !isWindowless {
		t.Errorf("got %q (windowless=%v), want %q", got, isWindowless, windowless)
	}

	if got, _ := wrapperSourceExe(console, true); got != console {
		t.Errorf("--console gave %q, want %q", got, console)
	}
}

func TestWrapperSourceExeFallsBackToSelf(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, executableName("exe-env-wrapper"))
	writeFile(t, self, "console only")

	got, windowless := wrapperSourceExe(self, false)
	if got != self || windowless {
		t.Errorf("got %q (windowless=%v), want %q and false", got, windowless, self)
	}
}

// The console build's own name contains "-windows-", which a substring match
// for "-w" reads as the windowless marker. This pins the distinction.
func TestIsWindowlessName(t *testing.T) {
	tests := map[string]bool{
		"exe-env-wrapper-w":               true,
		"exe-env-wrapper-w.exe":           true,
		"exe-env-wrapper-w-windows-amd64": true,
		"exe-env-wrapper-w-windows-arm64": true,
		"exe-env-wrapper":                 false,
		"exe-env-wrapper-windows-amd64":   false,
		"exe-env-wrapper-windows-arm64":   false,
		"exe-env-wrapper-linux-amd64":     false,
		"yt-dlp":                          false,
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isWindowlessName(name); got != want {
				t.Errorf("isWindowlessName(%q) = %v, want %v", name, got, want)
			}
		})
	}
}
