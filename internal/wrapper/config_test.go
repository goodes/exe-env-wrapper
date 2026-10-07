package wrapper

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigCandidatesFollowExeName(t *testing.T) {
	cases := []struct {
		exe  string
		stem string
	}{
		{filepath.Join("C:", "tools", "yt.exe"), "yt"},
		{filepath.Join("C:", "tools", "yt.EXE"), "yt"},
		{filepath.Join("C:", "tools", "foo.exe"), "foo"},
		{filepath.Join("/opt", "bin", "yt"), "yt"},
		{filepath.Join("/opt", "bin", "my.tool.exe"), "my.tool"},
	}
	for _, tc := range cases {
		t.Run(tc.exe, func(t *testing.T) {
			if got := BaseName(tc.exe); got != tc.stem {
				t.Fatalf("BaseName(%q) = %q, want %q", tc.exe, got, tc.stem)
			}
			got := ConfigCandidates(tc.exe)
			want := []string{
				filepath.Join(filepath.Dir(tc.exe), tc.stem+".yaml"),
				filepath.Join(filepath.Dir(tc.exe), tc.stem+".yml"),
			}
			if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Fatalf("ConfigCandidates(%q) = %q, want %q", tc.exe, got, want)
			}
		})
	}
}

func TestFindConfigPrefersYamlOverYml(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "yt"+exeSuffix())
	write(t, filepath.Join(dir, "yt.yml"), "target: a\n")

	got, err := FindConfig(exe)
	if err != nil {
		t.Fatalf("FindConfig with only .yml: %v", err)
	}
	if want := filepath.Join(dir, "yt.yml"); got != want {
		t.Fatalf("FindConfig = %q, want %q", got, want)
	}

	write(t, filepath.Join(dir, "yt.yaml"), "target: b\n")
	got, err = FindConfig(exe)
	if err != nil {
		t.Fatalf("FindConfig with both: %v", err)
	}
	if want := filepath.Join(dir, "yt.yaml"); got != want {
		t.Fatalf("FindConfig = %q, want %q", got, want)
	}
}

func TestFindConfigMissingNamesWhatItLookedFor(t *testing.T) {
	dir := t.TempDir()
	_, err := FindConfig(filepath.Join(dir, "yt.exe"))
	if err == nil {
		t.Fatal("FindConfig succeeded with no config present")
	}
	if !strings.Contains(err.Error(), "yt.yaml") || !strings.Contains(err.Error(), "yt.yml") {
		t.Fatalf("error should name both candidates, got %v", err)
	}
}

func TestLoadConfigOnlySeesItsOwnName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "yt.yaml"), "target: yt-cli\nenv:\n  A: \"1\"\n")
	write(t, filepath.Join(dir, "foo.yaml"), "target: foo-cli\n")

	cfg, path, err := LoadConfig(filepath.Join(dir, "foo"+exeSuffix()))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if path != filepath.Join(dir, "foo.yaml") {
		t.Fatalf("loaded %q, want foo.yaml", path)
	}
	if cfg.Target != "foo-cli" {
		t.Fatalf("target = %q, want foo-cli", cfg.Target)
	}
	if len(cfg.Env) != 0 {
		t.Fatalf("env = %v, want empty", cfg.Env)
	}
}

func TestParseConfigKeepsEnvOrder(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
target: yt-cli.exe
log: errors.log
env:
  HTTPS_PROXY: http://proxy.example:8080
  HTTP_PROXY: http://proxy.example:8080
  NO_PROXY: localhost,127.0.0.1
  PORT: 8080
  EMPTY:
`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Target != "yt-cli.exe" || cfg.Log != "errors.log" {
		t.Fatalf("target/log = %q/%q", cfg.Target, cfg.Log)
	}
	want := []EnvVar{
		{"HTTPS_PROXY", "http://proxy.example:8080"},
		{"HTTP_PROXY", "http://proxy.example:8080"},
		{"NO_PROXY", "localhost,127.0.0.1"},
		{"PORT", "8080"},
		{"EMPTY", ""},
	}
	if len(cfg.Env) != len(want) {
		t.Fatalf("env = %v, want %v", cfg.Env, want)
	}
	for i := range want {
		if cfg.Env[i] != want[i] {
			t.Fatalf("env[%d] = %v, want %v", i, cfg.Env[i], want[i])
		}
	}
}

func TestParseConfigRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"missing target":  "env:\n  A: 1\n",
		"empty target":    "target: \"\"\n",
		"env not mapping": "target: a\nenv:\n  - A=1\n",
		"env nested":      "target: a\nenv:\n  A:\n    B: 1\n",
		"broken yaml":     "target: [a\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(body)); err == nil {
				t.Fatalf("ParseConfig(%q) succeeded, want error", body)
			}
		})
	}
}

func TestResolveTargetRelativeToWrapperDir(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "yt"+exeSuffix())
	write(t, exe, "wrapper")
	sub := filepath.Join(dir, "bin")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "yt-cli"+exeSuffix())
	write(t, target, "target")

	// The working directory must not take part in resolution.
	chdir(t, t.TempDir())

	for _, spec := range []string{
		"bin/yt-cli" + exeSuffix(),
		filepath.Join("bin", "yt-cli"+exeSuffix()),
		"./bin/yt-cli" + exeSuffix(),
		target,
	} {
		got, err := ResolveTarget(spec, exe)
		if err != nil {
			t.Fatalf("ResolveTarget(%q): %v", spec, err)
		}
		if got != target {
			t.Fatalf("ResolveTarget(%q) = %q, want %q", spec, got, target)
		}
	}
}

func TestResolveTargetMissing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "yt"+exeSuffix())
	write(t, exe, "wrapper")

	_, err := ResolveTarget("nope", exe)
	if err == nil {
		t.Fatal("ResolveTarget succeeded for a missing target")
	}
	if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error should name the missing path, got %v", err)
	}
}

func TestResolveTargetRefusesItself(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "yt"+exeSuffix())
	write(t, exe, "wrapper")

	_, err := ResolveTarget("yt"+exeSuffix(), exe)
	if err == nil {
		t.Fatal("ResolveTarget pointed the wrapper at itself without complaint")
	}
	if !strings.Contains(err.Error(), "wrapper itself") {
		t.Fatalf("error should explain the loop, got %v", err)
	}
}

func TestResolvePathLeavesAbsoluteAlone(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator)+"var", "log", "yt.log")
	if got := ResolvePath(abs, filepath.Join("/opt", "bin")); got != abs {
		t.Fatalf("ResolvePath(%q) = %q, want it unchanged", abs, got)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}
