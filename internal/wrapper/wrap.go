package wrapper

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// wrapParams is everything --wrap needs, passed in rather than read from the
// environment so the whole command can be exercised against temp directories.
type wrapParams struct {
	// tool is the --wrap argument: a bare name to resolve on PATH, or a path.
	tool string
	// wrapperDir is where the wrapper is installed.
	wrapperDir string
	// sourceExe is the wrapper binary to copy in.
	sourceExe string
	// writeConfig forces a config file even when resolution stays dynamic.
	writeConfig bool
	// pathEntries and probe resolve a bare name.
	pathEntries []string
	probe       pathProbe
	out         io.Writer
}

// wrapTool installs a wrapper for a tool into the wrapper folder, leaving the
// original binary untouched.
//
// A bare name leaves resolution dynamic: the wrapper finds the tool on PATH at
// run time, so the tool can be upgraded, reinstalled or moved without
// invalidating anything. That is the whole point of wrapping rather than
// adapting. A path pins exactly one file, which has to be recorded in a config
// because it cannot be rediscovered.
func wrapTool(p wrapParams) error {
	name, realPath, pinned, err := resolveWrapTarget(p)
	if err != nil {
		return err
	}
	if err := checkWrappable(realPath); err != nil {
		return err
	}

	if err := os.MkdirAll(p.wrapperDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", p.wrapperDir, err)
	}

	dest := filepath.Join(p.wrapperDir, executableName(name))
	if samePathEntry(dest, realPath) {
		return fmt.Errorf("%s is already the wrapper; nothing to wrap", realPath)
	}
	if err := copyExecutable(p.sourceExe, dest); err != nil {
		return err
	}
	fmt.Fprintf(p.out, "installed %s\n", dest)
	fmt.Fprintf(p.out, "wraps     %s\n", realPath)

	if err := writeWrapConfig(p, name, realPath, pinned); err != nil {
		return err
	}

	reportWrapOrdering(p, realPath)
	return nil
}

// resolveWrapTarget works out the tool's name, the binary being wrapped, and
// whether the config has to pin it.
func resolveWrapTarget(p wrapParams) (name, realPath string, pinned bool, err error) {
	if isPathArgument(p.tool) {
		path, err := filepath.Abs(p.tool)
		if err != nil {
			return "", "", false, fmt.Errorf("%s: %w", p.tool, err)
		}
		if !isRegularFile(path) {
			return "", "", false, fmt.Errorf("not a file: %s", path)
		}
		return BaseName(path), path, true, nil
	}

	name = BaseName(p.tool)
	path, err := resolveNextOnPath(name, p.wrapperDir, p.pathEntries, p.probe)
	if err != nil {
		return "", "", false, fmt.Errorf("cannot wrap %s: %w", name, err)
	}
	return name, path, false, nil
}

// isPathArgument reports whether the argument names a location rather than a
// bare tool name.
func isPathArgument(arg string) bool {
	return strings.ContainsRune(arg, '/') || strings.ContainsRune(arg, os.PathSeparator) ||
		filepath.IsAbs(arg) || strings.Contains(arg, ":")
}

// checkWrappable refuses what the wrapper cannot stand in for.
func checkWrappable(path string) error {
	if isWrapperBinary(path) {
		return fmt.Errorf("%s is already an exe-env-wrapper", path)
	}
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe") {
		// A .cmd or .bat would have to be run through cmd.exe, which gives up
		// the raw command-line pass-through the wrapper exists to preserve.
		return fmt.Errorf("only .exe files can be wrapped, not %s", filepath.Ext(path))
	}
	return nil
}

// writeWrapConfig writes a config only when one is needed: always for a pinned
// path, otherwise only on request. The common case needs no config file at
// all, because a wrapper with none applies the built-in defaults.
func writeWrapConfig(p wrapParams, name, realPath string, pinned bool) error {
	path := filepath.Join(p.wrapperDir, name+".yaml")
	if !pinned && !p.writeConfig {
		fmt.Fprint(p.out, "config    none needed: built-in defaults, target resolved on PATH\n")
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(p.out, "config    %s left alone (already exists)\n", path)
		return nil
	}

	var b strings.Builder
	if pinned {
		fmt.Fprintf(&b, "target: %s\n", realPath)
	}
	b.WriteString("env:\n")
	b.WriteString("  HTTP_PROXY: ${system.http_proxy}\n")
	b.WriteString("  HTTPS_PROXY: ${system.https_proxy}\n")
	b.WriteString("  NO_PROXY: ${system.no_proxy}\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(p.out, "config    %s\n", path)
	return nil
}

// reportWrapOrdering says whether the wrapper will actually be reached. A
// wrapper behind the real tool on PATH is never invoked, and silently does
// nothing at all.
func reportWrapOrdering(p wrapParams, realPath string) {
	// Absent from PATH entirely is a different problem from being on it in the
	// wrong order, and needs a different instruction, so it is checked first
	// rather than inferred from the ordering walk.
	onPath := false
	for _, entry := range p.pathEntries {
		if samePathEntry(entry, p.wrapperDir) {
			onPath = true
			break
		}
	}
	if !onPath {
		fmt.Fprintf(p.out, "\nWARNING: %s is not on PATH, so the wrapper will never be\n", p.wrapperDir)
		fmt.Fprint(p.out, "reached. Run --setup to add it.\n")
		return
	}

	realDir := filepath.Dir(realPath)
	for _, entry := range p.pathEntries {
		if samePathEntry(entry, p.wrapperDir) {
			return // Reached before the real tool: nothing to warn about.
		}
		if samePathEntry(entry, realDir) {
			fmt.Fprintf(p.out, "\nWARNING: %s comes before %s on PATH,\n", entry, p.wrapperDir)
			fmt.Fprint(p.out, "so the wrapper will never be reached. Run --setup, or use --adapt.\n")
			return
		}
	}
}

// copyExecutable copies src to dest, replacing dest if it is there.
func copyExecutable(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	// Removed first: on Windows a running executable cannot be overwritten in
	// place, and replacing a wrapper while something holds it open is a
	// realistic thing to be doing.
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replacing %s: %w", dest, err)
	}
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return nil
}

// wrapperSourceExe picks the binary to install: this one, unless the
// windowless build is asked for by name.
//
// The console build is the default because it is the one that always works.
// Command-line tools are what gets wrapped, and their callers — a shell, or an
// agent running them as a shell — already own a console, which a
// console-subsystem child inherits, so nothing is allocated and no window
// appears. The same wrapper also still works when run by hand to debug it.
//
// The windowless build only earns its place under a host that has no console
// of its own and does not pass CREATE_NO_WINDOW, where a console-subsystem
// child would make Windows allocate one and flash a window. It is the worse
// default: from a shell it is not waited for, delivers no output, and reports
// the wrong exit status, because a shell decides whether to wait from the
// subsystem alone.
func wrapperSourceExe(selfPath string, wantWindowless bool) (path string, windowless bool, err error) {
	if !wantWindowless {
		return selfPath, false, nil
	}
	if sibling, ok := findWindowlessSibling(selfPath); ok {
		return sibling, true, nil
	}
	return "", false, fmt.Errorf(
		"no windowless build found beside %s; download %s-w-<platform>.exe into the same directory",
		filepath.Dir(selfPath), wrapperNamePrefix)
}

// isWindowlessName reports whether a base name is a windowless build.
//
// The "-w" has to be matched as a whole segment. A substring check for
// "exe-env-wrapper-w" also matches "exe-env-wrapper-windows-amd64", which is
// the console build, and would have the console build install itself while
// claiming to be windowless.
func isWindowlessName(base string) bool {
	lower := strings.ToLower(BaseName(base))
	if !strings.HasPrefix(lower, wrapperNamePrefix) {
		return false
	}
	rest := lower[len(wrapperNamePrefix):]
	return rest == "-w" || strings.HasPrefix(rest, "-w-")
}

// findWindowlessSibling looks for a windowless build beside selfPath.
func findWindowlessSibling(selfPath string) (string, bool) {
	if runtime.GOOS != "windows" {
		return "", false
	}
	if isWindowlessName(selfPath) {
		return selfPath, true // Already the windowless build.
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(selfPath), wrapperNamePrefix+"-w*.exe"))
	if err != nil {
		return "", false
	}
	for _, match := range matches {
		// The glob is deliberately loose, so each hit is checked properly.
		if isWindowlessName(match) {
			return match, true
		}
	}
	return "", false
}

// runWrap is the wrap command.
func runWrap(out io.Writer, tool string, opts map[string]bool) error {
	if tool == "" {
		return fmt.Errorf("wrap needs a tool name or path; try \"help wrap\"")
	}
	dir, err := wrapperFolder()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine own path: %w", err)
	}
	source, windowless, err := wrapperSourceExe(self, opts["--windowless"])
	if err != nil {
		return err
	}
	if windowless {
		fmt.Fprintf(out, "using    %s (windowless)\n", filepath.Base(source))
	}
	return wrapTool(wrapParams{
		tool:        tool,
		wrapperDir:  dir,
		sourceExe:   source,
		writeConfig: opts["--config"],
		pathEntries: effectivePathEntries(),
		probe:       realPathProbe(),
		out:         out,
	})
}
