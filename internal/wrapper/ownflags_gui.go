//go:build windowsgui

package wrapper

// ownFlagsEnabled is off in the windowless build: see the console variant.
// This build is a runtime payload, installed by the console build.
const ownFlagsEnabled = false
