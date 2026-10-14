package wrapper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Direction markers in a trace file. A stdio protocol is a conversation, and
// a transcript of it is unreadable without knowing who said what.
const (
	traceIn  = "<<"
	traceOut = ">>"
	traceErr = "!!"
)

// traceSink writes a transcript of the target's streams.
//
// Every write is best effort: a trace is a debugging aid, and a full disk or a
// revoked permission must not take down the tool being traced. The first
// failure is reported once and the rest are dropped.
type traceSink struct {
	mu       sync.Mutex
	w        io.WriteCloser
	reported bool
	stderr   io.Writer
	name     string
}

func newTraceSink(path, name string, stderr io.Writer) (*traceSink, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cannot write trace %s: %w", path, err)
	}
	return &traceSink{w: f, stderr: stderr, name: name}, nil
}

// record writes one framed chunk.
func (t *traceSink) record(direction string, data []byte) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.w == nil {
		return
	}
	stamp := time.Now().Format("15:04:05.000")
	for _, line := range splitKeepingPartial(data) {
		if _, err := fmt.Fprintf(t.w, "%s %s %s\n", stamp, direction, line); err != nil {
			if !t.reported {
				fmt.Fprintf(t.stderr, "%s: trace write failed, continuing untraced: %v\n", t.name, err)
				t.reported = true
			}
			t.w = nil
			return
		}
	}
}

func (t *traceSink) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.w != nil {
		_ = t.w.Close()
		t.w = nil
	}
}

// splitKeepingPartial breaks a chunk into display lines without discarding a
// trailing fragment, so nothing written is ever missing from the transcript.
func splitKeepingPartial(data []byte) []string {
	trimmed := bytes.TrimRight(data, "\r\n")
	if len(trimmed) == 0 {
		return nil
	}
	parts := bytes.Split(trimmed, []byte("\n"))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, string(bytes.TrimRight(part, "\r")))
	}
	return out
}

// teeStream copies src to dst, recording what passes through.
//
// The real stream is written first and the transcript second, so a slow or
// failing trace cannot delay a protocol message. Nothing waits for a newline:
// a stdio peer may be blocked on a reply that has not been terminated yet.
func teeStream(dst io.Writer, src io.Reader, sink *traceSink, direction string) error {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			sink.record(direction, buf[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

// execTargetTraced runs the target with its streams copied through the
// wrapper, so they can be transcribed.
//
// This is the path the default deliberately avoids. Without a trace the child
// inherits the standard handles directly and, on Unix, the wrapper is replaced
// by the target altogether; both mean nothing sits between the caller and the
// tool. Tracing gives that up, which is why it is opt-in.
func execTargetTraced(target string, args, env []string, sink *traceSink) (int, error) {
	cmd := exec.Command(target, args...)
	cmd.Env = env
	configureSpawn(cmd, target)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ExitError, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ExitError, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ExitError, err
	}

	if err := cmd.Start(); err != nil {
		return ExitError, err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = teeStream(os.Stdout, stdout, sink, traceOut)
	}()
	go func() {
		defer wg.Done()
		_ = teeStream(os.Stderr, stderr, sink, traceErr)
	}()
	// Not waited on: a tool that never reads stdin would otherwise keep the
	// wrapper alive after the tool itself had exited.
	go func() {
		_ = teeStream(stdin, os.Stdin, sink, traceIn)
		// The target has to see EOF, or a stdio server waits for a request
		// that will never come.
		_ = stdin.Close()
	}()

	// Both output streams are drained before Wait, so nothing written just
	// before exit is lost from either the terminal or the transcript.
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return ExitError, err
	}
	return 0, nil
}

// recordLine writes one line of the wrapper's own commentary, as distinct from
// the stream contents the direction markers carry.
func (t *traceSink) recordLine(message string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.w == nil {
		return
	}
	if _, err := fmt.Fprintf(t.w, "%s -- %s\n", time.Now().Format("15:04:05.000"), message); err != nil {
		if !t.reported {
			fmt.Fprintf(t.stderr, "%s: trace write failed, continuing untraced: %v\n", t.name, err)
			t.reported = true
		}
		t.w = nil
	}
}
