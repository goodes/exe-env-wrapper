package wrapper

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// childSource is a stand-in for the wrapped tool. It reports the arguments and
// the environment it was handed, echoes stdin, and exits with the status named
// by CHILD_EXIT.
const childSource = `package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
)

func main() {
	for _, a := range os.Args[1:] {
		fmt.Printf("ARG:%s\n", a)
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "INHERITED", "COMPOSED"} {
		if v, ok := os.LookupEnv(name); ok {
			fmt.Printf("ENV:%s=%s\n", name, v)
		}
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		fmt.Printf("IN:%s\n", in.Text())
	}
	fmt.Fprintln(os.Stderr, "ERR:from child")
	if code, err := strconv.Atoi(os.Getenv("CHILD_EXIT")); err == nil {
		os.Exit(code)
	}
}
`

const childModule = "module child\n\ngo 1.22\n"

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Skip("not in a module")
	}
	return filepath.Dir(gomod)
}

func goBuild(t *testing.T, dir, pkg, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, combined)
	}
}

// install builds the wrapper under wrapperName beside a freshly built child,
// writes config, and returns the directory and the wrapper's path.
func install(t *testing.T, wrapperName, config string) (dir, wrapperPath string) {
	t.Helper()
	root := repoRoot(t)
	dir = t.TempDir()

	childDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(childDir, "main.go"), []byte(childSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, childDir, ".", filepath.Join(dir, "yt-cli"+exeSuffix()))

	wrapperPath = filepath.Join(dir, wrapperName+exeSuffix())
	goBuild(t, root, "./cmd/exe-env-wrapper", wrapperPath)

	if config != "" {
		write(t, filepath.Join(dir, wrapperName+".yaml"), config)
	}
	return dir, wrapperPath
}

type result struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, wrapperPath, stdin string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(wrapperPath, args...)
	// A directory unrelated to the wrapper, to prove relative targets resolve
	// against the wrapper's own location.
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	res := result{stdout: out.String(), stderr: errOut.String()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		res.code = exitErr.ExitCode()
	default:
		t.Fatalf("running wrapper: %v\nstderr: %s", err, res.stderr)
	}
	return res
}

func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	_, wrapper := install(t, "yt", `
target: yt-cli`+exeSuffix()+`
env:
  HTTP_PROXY: http://proxy.example:8080
  HTTPS_PROXY: ${HTTP_PROXY}
  NO_PROXY: localhost,127.0.0.1
  COMPOSED: "<${INHERITED}>"
`)

	args := []string{"-x", "--output", "a b c", `say "hi"`, "", "tail$arg"}
	res := run(t, wrapper, "line one\nline two\n", []string{"INHERITED=kept", "CHILD_EXIT=7"}, args...)

	if res.code != 7 {
		t.Fatalf("exit code = %d, want the target's 7\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	got := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
	want := []string{
		"ARG:-x", "ARG:--output", "ARG:a b c", `ARG:say "hi"`, "ARG:", "ARG:tail$arg",
		"ENV:HTTP_PROXY=http://proxy.example:8080",
		"ENV:HTTPS_PROXY=http://proxy.example:8080",
		"ENV:NO_PROXY=localhost,127.0.0.1",
		"ENV:INHERITED=kept",
		"ENV:COMPOSED=<kept>",
		"IN:line one",
		"IN:line two",
	}
	if len(got) != len(want) {
		t.Fatalf("stdout lines = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stdout line %d = %q, want %q", i, got[i], want[i])
		}
	}
	if !strings.Contains(res.stderr, "ERR:from child") {
		t.Fatalf("stderr = %q, want the child's stderr passed through", res.stderr)
	}
}

func TestEndToEndSuccessExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	_, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\n")
	if res := run(t, wrapper, "", nil); res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
}

func TestEndToEndConfigByWrapperName(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	// The same build under a different name reads a different config.
	dir, wrapper := install(t, "foo", "target: yt-cli"+exeSuffix()+"\nenv:\n  HTTP_PROXY: http://foo:1\n")
	write(t, filepath.Join(dir, "yt.yaml"), "target: yt-cli"+exeSuffix()+"\nenv:\n  HTTP_PROXY: http://yt:2\n")

	res := run(t, wrapper, "", nil)
	if !strings.Contains(res.stdout, "ENV:HTTP_PROXY=http://foo:1") {
		t.Fatalf("stdout = %q, want foo.yaml's proxy", res.stdout)
	}
}

func TestEndToEndMissingConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	_, wrapper := install(t, "yt", "")
	res := run(t, wrapper, "", nil)
	if res.code != ExitError {
		t.Fatalf("exit code = %d, want %d", res.code, ExitError)
	}
	if !strings.Contains(res.stderr, "yt.yaml") || !strings.Contains(res.stderr, "no config file") {
		t.Fatalf("stderr = %q, want a message naming the missing config", res.stderr)
	}
}

func TestEndToEndMissingTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	_, wrapper := install(t, "yt", "target: not-there"+exeSuffix()+"\n")
	res := run(t, wrapper, "", nil)
	if res.code != ExitError {
		t.Fatalf("exit code = %d, want %d", res.code, ExitError)
	}
	if !strings.Contains(res.stderr, "target not found") {
		t.Fatalf("stderr = %q, want a target-not-found message", res.stderr)
	}
}

func TestEndToEndWritesLogFile(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	dir, wrapper := install(t, "yt", "target: not-there\nlog: errors.log\n")
	if res := run(t, wrapper, "", nil); res.code != ExitError {
		t.Fatalf("exit code = %d, want %d", res.code, ExitError)
	}
	body, err := os.ReadFile(filepath.Join(dir, "errors.log"))
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(body), "target not found") {
		t.Fatalf("log = %q, want the failure recorded", body)
	}
}

func TestEndToEndBootstrapLogEnvVar(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	dir, wrapper := install(t, "yt", "")
	logPath := filepath.Join(dir, "bootstrap.log")
	if res := run(t, wrapper, "", []string{LogEnvVar + "=" + logPath}); res.code != ExitError {
		t.Fatalf("exit code = %d, want %d", res.code, ExitError)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(body), "no config file") {
		t.Fatalf("log = %q, want the config failure recorded", body)
	}
}

func TestEndToEndAbsoluteTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	dir, wrapper := install(t, "yt", "")
	write(t, filepath.Join(dir, "yt.yaml"), "target: "+filepath.Join(dir, "yt-cli"+exeSuffix())+"\n")
	if res := run(t, wrapper, "", nil); res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
}
