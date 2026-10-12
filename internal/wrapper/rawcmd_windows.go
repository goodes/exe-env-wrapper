//go:build windows

package wrapper

// rawCommandTail is the caller's own argument text, which is what the target
// actually receives on Windows, so it is what the log should show.
func rawCommandTail() string { return rawArgTail() }
