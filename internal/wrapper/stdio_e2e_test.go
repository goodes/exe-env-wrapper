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

// stdioChildSource is a stand-in for a stdio server of the sort MCP uses: it
// reads a line, answers it immediately, and keeps going until stdin closes.
// It writes its own diagnostics to stderr, where a real one does, so that
// stdout carries nothing but protocol.
const stdioChildSource = `package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "stdio child ready")
	in := bufio.NewScanner(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		fmt.Fprintf(out, "PONG:%s\n", in.Text())
		// A stdio server has to flush per message: the peer is waiting for
		// this reply before it sends anything else.
		out.Flush()
	}
	fmt.Fprintln(os.Stderr, "stdio child done")
}
`

// installStdioWrapper lays out a wrapper over the stdio child.
func installStdioWrapper(t *testing.T, config string) string {
	t.Helper()
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
	write(t, filepath.Join(dir, "server.yaml"), config)
	return wrapperPath
}

// A stdio server is driven request by request: the peer writes a message and
// waits for the reply before sending the next. If the wrapper buffered stdout,
// or sat between the two copying with its own buffer, the first read would
// block and the server would appear to hang.
func TestEndToEndStdioRoundTrips(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	wrapperPath := installStdioWrapper(t, "target: server-cli"+exeSuffix()+"\nenv:\n  NO_PROXY: ${system.no_proxy}\n")

	cmd := exec.Command(wrapperPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the wrapper: %v", err)
	}

	replies := bufio.NewScanner(stdout)
	for _, message := range []string{"initialize", "tools/list", "tools/call", "shutdown"} {
		if _, err := io.WriteString(stdin, message+"\n"); err != nil {
			t.Fatalf("writing %q: %v", message, err)
		}

		// A reply has to arrive before the next message is sent. The deadline
		// is what turns "buffered until exit" into a failure rather than a
		// hang, since nothing further will ever be written.
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
			t.Fatalf("no reply to %q within 10s; stdout is buffered rather than streamed\nstderr: %s",
				message, stderr.String())
		}
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("closing stdin: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the wrapper did not exit cleanly after stdin closed: %v\nstderr: %s", err, stderr.String())
	}

	// The child's own diagnostics must have reached stderr, which is where a
	// stdio server puts anything that is not protocol.
	if !strings.Contains(stderr.String(), "stdio child ready") {
		t.Errorf("stderr = %q, want the child's diagnostics", stderr.String())
	}
}

// Nothing the wrapper has to say may go to stdout. For a stdio server that
// stream is the protocol, and one stray line of wrapper output corrupts it.
func TestEndToEndStdioStdoutCarriesOnlyProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	// A config with no target, so the wrapper also resolves from PATH, plus a
	// log file, which is the other thing that might tempt it to narrate.
	wrapperPath := installStdioWrapper(t, "target: server-cli"+exeSuffix()+"\nlog: wrapper.log\n")

	cmd := exec.Command(wrapperPath)
	cmd.Stdin = strings.NewReader("one\ntwo\n")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running the wrapper: %v\nstderr: %s", err, stderr.String())
	}

	want := "PONG:one\nPONG:two\n"
	got := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	if got != want {
		t.Errorf("stdout = %q, want exactly %q with nothing added", got, want)
	}
}

// The exit status has to survive, because a peer uses it to tell a clean
// shutdown from a crash.
func TestEndToEndStdioPropagatesExitStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	dir := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(childSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, src, ".", filepath.Join(dir, "server-cli"+exeSuffix()))

	wrapperPath := filepath.Join(dir, "server"+exeSuffix())
	goBuild(t, repoRoot(t), "./cmd/exe-env-wrapper", wrapperPath)
	write(t, filepath.Join(dir, "server.yaml"), "target: server-cli"+exeSuffix()+"\n")

	res := run(t, wrapperPath, "", []string{"CHILD_EXIT=23"})
	if res.code != 23 {
		t.Errorf("exit code = %d, want 23 from the server", res.code)
	}
}
