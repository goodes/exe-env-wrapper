//go:build !windows

package wrapper

// rawCommandTail has no equivalent here: a Unix process receives an argument
// vector, not a command line, so there is no original text to reproduce.
func rawCommandTail() string { return "" }
