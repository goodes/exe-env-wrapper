package wrapper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// executableName is base as an executable file name on this platform.
func executableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// pathProbe is the filesystem resolution asks about, injected so the ordering
// rules can be tested without laying out directories.
type pathProbe struct {
	// sameDir reports whether two PATH entries denote the same directory.
	sameDir func(a, b string) bool
	// isFile reports whether a path is an existing regular file.
	isFile func(path string) bool
	// isWrapper reports whether a path is an exe-env-wrapper build.
	isWrapper func(path string) bool
}

// realPathProbe answers from the actual filesystem.
func realPathProbe() pathProbe {
	return pathProbe{
		sameDir:   sameDirectory,
		isFile:    isRegularFile,
		isWrapper: isWrapperBinary,
	}
}

// resolveNextOnPath finds the executable that would have been run if this
// wrapper were not on PATH: the first match for name in a PATH directory other
// than the wrapper's own.
//
// This is what lets a wrapper in the wrapper folder shadow a tool without
// knowing where the tool lives. An installer may regenerate the real binary,
// or move it to a different directory, and resolution still finds it.
func resolveNextOnPath(name, ownDir string, entries []string, probe pathProbe) (string, error) {
	wanted := executableName(name)
	var searched []string

	for _, dir := range entries {
		if dir == "" {
			continue
		}
		// Compared as directories rather than as strings: PATH routinely lists
		// one directory under several spellings, and a string comparison would
		// let the wrapper resolve to itself.
		if probe.sameDir(dir, ownDir) {
			continue
		}
		searched = append(searched, dir)

		candidate := filepath.Join(dir, wanted)
		if !probe.isFile(candidate) {
			continue
		}
		// Another wrapper is not a target. Handing off to one would build
		// indirection at best, and a loop between two of them at worst.
		if probe.isWrapper(candidate) {
			continue
		}
		return candidate, nil
	}

	return "", fmt.Errorf("no %s found on PATH outside %s; searched %s",
		wanted, ownDir, summarizeDirs(searched))
}

// maxDirsInError is how many directories an error names before summarising.
// A full PATH runs to a couple of dozen entries, and a message that dumps all
// of them buries the part that matters.
const maxDirsInError = 6

// summarizeDirs renders the searched directories for an error message, naming
// the first few and counting the rest. --check reports the full picture.
func summarizeDirs(dirs []string) string {
	if len(dirs) <= maxDirsInError {
		return strings.Join(dirs, ", ")
	}
	return fmt.Sprintf("%s and %d more",
		strings.Join(dirs[:maxDirsInError], ", "), len(dirs)-maxDirsInError)
}

// sameDirectory reports whether two PATH entries denote the same directory,
// by identity where the filesystem can say so and by cleaned path otherwise.
func sameDirectory(a, b string) bool {
	if a == b {
		return true
	}
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

// isRegularFile reports whether path exists and is not a directory.
func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// pathEntries splits a PATH value into its directories.
func pathEntries(value string) []string {
	return filepath.SplitList(value)
}

// resolveTargetFor picks what to run: the target the config names, or, when it
// names none, the next match for the wrapper's own name on PATH.
func resolveTargetFor(cfg *Config, exePath string) (string, error) {
	return resolveTargetForWith(cfg, exePath, pathEntries(os.Getenv("PATH")), realPathProbe())
}

// resolveTargetForWith is resolveTargetFor with PATH and the filesystem passed
// in, so --check can report what a wrapper would resolve to without being that
// wrapper.
func resolveTargetForWith(cfg *Config, exePath string, entries []string, probe pathProbe) (string, error) {
	if cfg.Target != "" {
		return ResolveTarget(cfg.Target, exePath)
	}
	return resolveNextOnPath(BaseName(exePath), filepath.Dir(exePath), entries, probe)
}
