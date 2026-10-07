//go:build windows && !windowsgui

package wrapper

// creationFlags is zero for the console build: run from a terminal, the child
// shares the console it inherits, so interactive output and progress bars work
// as if the wrapper were not there.
const creationFlags uint32 = 0
