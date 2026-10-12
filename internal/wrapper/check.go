package wrapper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// checkParams is everything --check needs.
type checkParams struct {
	wrapperDir  string
	pathEntries []string
	probe       pathProbe
	out         io.Writer
}

// checkWrappers reports what each installed wrapper resolves to, and returns
// how many have something wrong with them.
//
// This is the answer to "this stopped working and I cannot see why". The
// failure modes are all invisible in normal use: a wrapper that is never
// reached because PATH moved, one whose tool has gone, one whose config stopped
// parsing.
func checkWrappers(p checkParams) (problems int, err error) {
	entries, err := os.ReadDir(p.wrapperDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(p.out, "no wrapper folder at %s; run --setup\n", p.wrapperDir)
			return 1, nil
		}
		return 0, fmt.Errorf("reading %s: %w", p.wrapperDir, err)
	}

	fmt.Fprintf(p.out, "wrapper folder: %s\n", p.wrapperDir)
	onPath := false
	for _, entry := range p.pathEntries {
		if samePathEntry(entry, p.wrapperDir) {
			onPath = true
			break
		}
	}
	if onPath {
		ahead := entriesAheadOf(p.pathEntries, p.wrapperDir)
		fmt.Fprintf(p.out, "on PATH at position %d of %d\n\n", len(ahead)+1, len(p.pathEntries))
	} else {
		fmt.Fprint(p.out, "NOT on PATH: no wrapper here is ever reached. Run --setup.\n\n")
		problems++
	}

	found := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(p.wrapperDir, entry.Name())
		if !isWrapperBinary(path) {
			continue
		}
		found++
		if !checkOneWrapper(path, p) {
			problems++
		}
	}
	if found == 0 {
		fmt.Fprint(p.out, "no wrappers installed; use --wrap <tool>\n")
	}
	return problems, nil
}

// checkOneWrapper reports on a single installed wrapper, returning whether it
// looks healthy.
func checkOneWrapper(path string, p checkParams) bool {
	name := BaseName(path)
	fmt.Fprintf(p.out, "%s\n", name)

	cfg, cfgPath, err := LoadConfig(path)
	switch {
	case err != nil && !isNoConfig(err):
		fmt.Fprintf(p.out, "  config  BROKEN: %v\n", err)
		return false
	case isNoConfig(err):
		cfg = DefaultConfig()
		fmt.Fprint(p.out, "  config  none: built-in defaults\n")
	default:
		fmt.Fprintf(p.out, "  config  %s\n", filepath.Base(cfgPath))
	}

	target, err := resolveTargetForWith(cfg, path, p.pathEntries, p.probe)
	if err != nil {
		fmt.Fprintf(p.out, "  target  UNRESOLVED: %v\n", err)
		return false
	}
	fmt.Fprintf(p.out, "  target  %s\n", target)
	return true
}

// isNoConfig reports whether err is the absent-config case.
func isNoConfig(err error) bool {
	return errors.Is(err, ErrNoConfig)
}

// runCheck is the --check command. A problem found is a non-zero status, so it
// is usable from a script.
func runCheck(out io.Writer) int {
	dir, err := wrapperFolder()
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 1
	}
	problems, err := checkWrappers(checkParams{
		wrapperDir:  dir,
		pathEntries: effectivePathEntries(),
		probe:       realPathProbe(),
		out:         out,
	})
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 1
	}
	if problems > 0 {
		fmt.Fprintf(out, "\n%d problem%s found\n", problems, plural(problems, "", "s"))
		return 1
	}
	return 0
}
