//go:build !windowsgui

package wrapper

// ownFlagsEnabled turns on the wrapper's own flags. The console build has them;
// the windowless build does not, because it has no console to print to and
// installing is an interactive act. That also saves it needing AttachConsole.
const ownFlagsEnabled = true
