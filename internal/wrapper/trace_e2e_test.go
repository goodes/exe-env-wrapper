package wrapper

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The command line moved out of the log and into trace level 1 in v0.3.3, so
// what the log records is covered by TestEndToEndLogCarriesOnlyFailures.

// Without a log configured nothing is written anywhere, so the common case
// still touches no files.
func TestEndToEndNoLogWritesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\n")
	if res := run(t, wrapper, "", nil); res.code != 0 {
		t.Fatalf("exit code = %d, want 0", res.code)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			t.Errorf("a log file was written without one being configured: %s", entry.Name())
		}
	}
}

// trace captures both directions of the conversation and the target's stderr.
func TestEndToEndTraceCapturesBothDirections(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(stdioChildSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, src, ".", filepath.Join(dir, "server-cli"+exeSuffix()))

	wrapperPath := filepath.Join(dir, "server"+exeSuffix())
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	// Level 2: the streams. Level 1, the default, records only the command.
	write(t, filepath.Join(dir, "server.yaml"),
		"target: server-cli"+exeSuffix()+"\ntrace: server-trace.log\ntrace_level: 2\n")

	cmd := exec.Command(wrapperPath)
	cmd.Stdin = strings.NewReader("initialize\ntools/list\n")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running the wrapper: %v\nstderr: %s", err, stderr.String())
	}

	// The real streams must be unchanged by being traced.
	if got := strings.ReplaceAll(stdout.String(), "\r\n", "\n"); got != "PONG:initialize\nPONG:tools/list\n" {
		t.Errorf("stdout = %q, want the protocol unaltered", got)
	}

	body, err := os.ReadFile(filepath.Join(dir, "server-trace.log"))
	if err != nil {
		t.Fatalf("reading the trace: %v", err)
	}
	trace := string(body)
	for _, want := range []string{
		traceIn + " initialize",
		traceOut + " PONG:initialize",
		traceIn + " tools/list",
		traceOut + " PONG:tools/list",
		traceErr + " stdio child ready",
	} {
		if !strings.Contains(trace, want) {
			t.Errorf("trace does not contain %q:\n%s", want, trace)
		}
	}
}

// The combination that matters: a traced stdio server must still answer each
// request before the next is sent. A trace that buffered, or that was written
// before the real stream, would stall the conversation.
func TestEndToEndTracedStdioStillRoundTrips(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(stdioChildSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, src, ".", filepath.Join(dir, "server-cli"+exeSuffix()))

	wrapperPath := filepath.Join(dir, "server"+exeSuffix())
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	write(t, filepath.Join(dir, "server.yaml"),
		"target: server-cli"+exeSuffix()+"\ntrace: server-trace.log\ntrace_level: 2\n")

	cmd := exec.Command(wrapperPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	replies := bufio.NewScanner(stdout)
	for _, message := range []string{"one", "two", "three", "four", "five"} {
		if _, err := io.WriteString(stdin, message+"\n"); err != nil {
			t.Fatalf("writing %q: %v", message, err)
		}
		got := make(chan string, 1)
		go func() {
			if replies.Scan() {
				got <- replies.Text()
			} else {
				got <- ""
			}
		}()
		select {
		case reply := <-got:
			if want := "PONG:" + message; reply != want {
				t.Fatalf("reply = %q, want %q", reply, want)
			}
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("traced stdio stalled on %q: the trace is in the data path", message)
		}
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the traced wrapper did not exit cleanly: %v", err)
	}
}

// A trace that cannot be opened must not take the tool down with it.
func TestEndToEndUnwritableTraceStillRunsTheTool(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	// A path whose parent directory does not exist cannot be created.
	_, wrapper := install(t, "yt",
		"target: yt-cli"+exeSuffix()+"\ntrace: no-such-dir/trace.log\n")

	res := run(t, wrapper, "", nil, "an-arg")
	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0: an unwritable trace must not fail the run\nstderr: %s",
			res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ARG:an-arg") {
		t.Errorf("stdout = %q, want the tool to have run anyway", res.stdout)
	}
	if !strings.Contains(res.stderr, "trace") {
		t.Errorf("stderr = %q, want the trace problem reported", res.stderr)
	}
}
