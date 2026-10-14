package wrapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installTraced lays out a wrapper over the stdio child with a trace at the
// given level, and returns the directory and the trace path.
func installTraced(t *testing.T, config string) (dir, wrapperPath, tracePath string) {
	t.Helper()
	dir = t.TempDir()

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(stdioChildSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, src, ".", filepath.Join(dir, "server-cli"+exeSuffix()))

	wrapperPath = filepath.Join(dir, "server"+exeSuffix())
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	write(t, filepath.Join(dir, "server.yaml"), config)
	return dir, wrapperPath, filepath.Join(dir, "server-trace.log")
}

const tracedEnvConfig = "env:\n  NO_PROXY: fixed.example\n  HTTPS_PROXY: ${system.https_proxy}\n"

// Each level has to contain what it promises and nothing from the level above,
// or the levels are decoration.
func TestEndToEndTraceLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	base := "target: server-cli" + exeSuffix() + "\ntrace: server-trace.log\n" + tracedEnvConfig

	tests := []struct {
		name      string
		config    string
		wantLines []string
		denyLines []string
	}{
		{
			name:      "level 1 records the command and nothing else",
			config:    base + "trace_level: 1\n",
			wantLines: []string{"run ", "server-cli"},
			denyLines: []string{">> PONG", "<< ping", "env NO_PROXY"},
		},
		{
			name:      "level 2 adds the streams",
			config:    base + "trace_level: 2\n",
			wantLines: []string{"run ", "<< ping", ">> PONG:ping"},
			denyLines: []string{"env NO_PROXY"},
		},
		{
			name:   "level 3 adds the variables changed",
			config: base + "trace_level: 3\n",
			wantLines: []string{
				"run ", "<< ping", ">> PONG:ping",
				`env NO_PROXY: (unset) -> "fixed.example"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, wrapper, tracePath := installTraced(t, tt.config)

			res := run(t, wrapper, "ping\n", nil, "--some-arg")
			if res.code != 0 {
				t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
			}

			body, err := os.ReadFile(tracePath)
			if err != nil {
				t.Fatalf("reading the trace: %v", err)
			}
			trace := string(body)

			for _, want := range tt.wantLines {
				if !strings.Contains(trace, want) {
					t.Errorf("trace does not contain %q:\n%s", want, trace)
				}
			}
			for _, deny := range tt.denyLines {
				if strings.Contains(trace, deny) {
					t.Errorf("trace contains %q, which belongs to a higher level:\n%s", deny, trace)
				}
			}
		})
	}
}

// The default when a trace file is named but no level is: level 1. That keeps
// the wrapper out of the data path unless asked for.
func TestEndToEndTraceDefaultsToLevelOne(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper, tracePath := installTraced(t,
		"target: server-cli"+exeSuffix()+"\ntrace: server-trace.log\n")

	if res := run(t, wrapper, "ping\n", nil); res.code != 0 {
		t.Fatalf("exit code = %d, want 0", res.code)
	}

	body, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("reading the trace: %v", err)
	}
	trace := string(body)
	if !strings.Contains(trace, "run ") {
		t.Errorf("trace does not record the command:\n%s", trace)
	}
	if strings.Contains(trace, ">> PONG") {
		t.Errorf("trace captured streams, so the default is not level 1:\n%s", trace)
	}
}

// log carries failures only. The command line moved to trace level 1, and a
// successful run must leave the log untouched.
func TestEndToEndLogCarriesOnlyFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	t.Run("a successful run writes nothing", func(t *testing.T) {
		dir, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\nlog: yt.log\n")
		if res := run(t, wrapper, "", nil, "-f", "bestaudio"); res.code != 0 {
			t.Fatalf("exit code = %d, want 0", res.code)
		}
		if _, err := os.Stat(filepath.Join(dir, "yt.log")); !os.IsNotExist(err) {
			body, _ := os.ReadFile(filepath.Join(dir, "yt.log"))
			t.Errorf("a log was written for a successful run:\n%s", body)
		}
	})

	t.Run("a failure is recorded", func(t *testing.T) {
		dir, wrapper := install(t, "yt", "target: not-there"+exeSuffix()+"\nlog: yt.log\n")
		if res := run(t, wrapper, "", nil); res.code != ExitError {
			t.Fatalf("exit code = %d, want %d", res.code, ExitError)
		}
		body, err := os.ReadFile(filepath.Join(dir, "yt.log"))
		if err != nil {
			t.Fatalf("reading the log: %v", err)
		}
		if !strings.Contains(string(body), "target not found") {
			t.Errorf("log = %q, want the failure recorded", body)
		}
	})
}

// A default that expanded to nothing is reported as left alone, rather than
// as having been set to empty.
//
// Checked on the renderer rather than end to end, because the case cannot be
// reached through a config file: only the built-in defaults decline a value,
// and those apply only when there is no config file, which is also the only
// place a trace can be switched on. See the note in the README.
func TestDescribeEnvChangeOnADeclinedDefault(t *testing.T) {
	got := describeEnvChange(EnvChange{Name: "HTTPS_PROXY", Applied: false})
	if !strings.Contains(got, "left as (unset)") {
		t.Errorf("describeEnvChange = %q, want it to say the variable was left alone", got)
	}
	if !strings.Contains(got, "expanded to nothing") {
		t.Errorf("describeEnvChange = %q, want it to say why", got)
	}

	got = describeEnvChange(EnvChange{Name: "HTTPS_PROXY", Before: "http://old:1", HadBefore: true})
	if !strings.Contains(got, `left as "http://old:1"`) {
		t.Errorf("describeEnvChange = %q, want the inherited value it kept", got)
	}
}

// A value written in a config file is applied even when it comes to nothing,
// because setting a variable empty on purpose has to keep working.
func TestEndToEndTraceLevel3ShowsAnEmptyConfiguredValueAsApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	config := "target: server-cli" + exeSuffix() + "\n" +
		"trace: server-trace.log\n" +
		"trace_level: 3\n" +
		"env:\n" +
		`  NO_PROXY: ""` + "\n"
	_, wrapper, tracePath := installTraced(t, config)

	if res := run(t, wrapper, "ping\n", []string{"NO_PROXY=inherited.example"}); res.code != 0 {
		t.Fatalf("exit code = %d, want 0", res.code)
	}

	body, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("reading the trace: %v", err)
	}
	if want := `env NO_PROXY: "inherited.example" -> ""`; !strings.Contains(string(body), want) {
		t.Errorf("trace does not contain %q:\n%s", want, body)
	}
}

// Level 3 shows an override's previous value, which is the other half of
// "why is this not what I configured".
func TestEndToEndTraceLevel3ShowsBeforeAndAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper, tracePath := installTraced(t,
		"target: server-cli"+exeSuffix()+
			"\ntrace: server-trace.log\ntrace_level: 3\nenv:\n  NO_PROXY: new.example\n")

	res := run(t, wrapper, "ping\n", []string{"NO_PROXY=old.example"})
	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}

	body, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("reading the trace: %v", err)
	}
	if want := `env NO_PROXY: "old.example" -> "new.example"`; !strings.Contains(string(body), want) {
		t.Errorf("trace does not contain %q:\n%s", want, body)
	}
}

// The case the environment variables exist for: wrap installs no config file,
// so without them there would be no way to trace the installation wrap
// actually produces.
func TestEndToEndTraceFromTheEnvironmentWithNoConfigFile(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	wrapperDir := t.TempDir()
	realDir := t.TempDir()
	buildChildAs(t, filepath.Join(realDir, executableName("yt")))

	wrapperPath := filepath.Join(wrapperDir, executableName("yt"))
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	// Deliberately no yt.yaml: this is what wrap installs.

	tracePath := filepath.Join(wrapperDir, "from-env.log")
	res := run(t, wrapperPath, "", []string{
		"PATH=" + wrapperDir + string(os.PathListSeparator) + realDir,
		TraceEnvVar + "=" + tracePath,
		TraceLevelEnvVar + "=3",
	}, "an-arg")
	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}

	body, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("reading the trace: %v", err)
	}
	trace := string(body)
	if !strings.Contains(trace, "run ") {
		t.Errorf("trace does not record the command:\n%s", trace)
	}
	// Level 3 on a config-less wrapper is the one place a declined default can
	// be seen: the built-in defaults apply only when there is no config file.
	if !strings.Contains(trace, "env ") {
		t.Errorf("trace does not report the variables:\n%s", trace)
	}
	if !strings.Contains(trace, "expanded to nothing") {
		t.Errorf("trace does not report the declined defaults:\n%s", trace)
	}
}

// A trace level with nowhere to write is reported, and the tool still runs.
func TestEndToEndTraceLevelWithoutAFileWarnsAndRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\n")
	res := run(t, wrapper, "", []string{TraceLevelEnvVar + "=2"}, "an-arg")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ARG:an-arg") {
		t.Errorf("stdout = %q, want the tool to have run", res.stdout)
	}
	if !strings.Contains(res.stderr, "nowhere") {
		t.Errorf("stderr = %q, want the misconfiguration reported", res.stderr)
	}
}
