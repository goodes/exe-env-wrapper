package wrapper

import (
	"debug/pe"
	"runtime"
)

// The two Windows builds.
const (
	variantConsole    = "console"
	variantWindowless = "windowless"
)

// Windows subsystem values from the PE optional header.
const (
	subsystemWindowsGUI = 2
	subsystemWindowsCUI = 3
)

// buildVariant is what this binary is.
func buildVariant() string {
	if ownFlagsEnabled {
		return variantConsole
	}
	return variantWindowless
}

// readVariantFromFile reports which of the two builds a wrapper binary is.
//
// Read from the PE subsystem field rather than from a stamped string. A stamp
// was tried first and failed twice over: the linker drops a string nothing
// reachable references, and it dropped it from precisely the windowless build
// that needed identifying, because the only code that read it lives behind the
// command mode that build does not have. Worse, the needle and the two names
// are separate constants that the linker parks next to each other, so a scan
// for the needle found "console" immediately after it in both builds.
//
// The subsystem field is what actually makes the two builds differ, so it
// cannot be dropped, cannot drift from the truth, and needs no cooperation
// from the thing being inspected.
func readVariantFromFile(path string) (string, bool) {
	f, err := pe.Open(path)
	if err != nil {
		// Not a PE file: a Linux binary, or something else entirely. There is
		// only one build on other platforms.
		if runtime.GOOS != "windows" {
			return variantConsole, true
		}
		return "", false
	}
	defer f.Close()

	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return variantFromSubsystem(h.Subsystem)
	case *pe.OptionalHeader64:
		return variantFromSubsystem(h.Subsystem)
	}
	return "", false
}

func variantFromSubsystem(subsystem uint16) (string, bool) {
	switch subsystem {
	case subsystemWindowsGUI:
		return variantWindowless, true
	case subsystemWindowsCUI:
		return variantConsole, true
	}
	return "", false
}
