package wrapper

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// pathStore is a persistent PATH list, behind a seam so the ordering logic can
// be exercised without writing to the real environment.
type pathStore struct {
	// scope names the store in messages: "user" or "machine".
	scope string
	get   func() ([]string, error)
	set   func([]string) error
}

// setupWrapperFolder creates the wrapper folder and puts it at the front of
// store, reporting what it did and what it still cannot shadow.
//
// effective is the PATH as a process actually sees it, used only to explain
// which directories sit ahead of the wrapper folder. On Windows that ordering
// is not something prepending to the user list can fix: every machine entry
// precedes every user entry, whatever their internal order.
func setupWrapperFolder(dir string, store pathStore, effective []string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	fmt.Fprintf(out, "wrapper folder: %s\n", dir)

	entries, err := store.get()
	if err != nil {
		return err
	}
	updated, changed := prependDir(entries, dir)
	switch {
	case !changed:
		fmt.Fprintf(out, "already first in the %s PATH, left alone\n", store.scope)
	default:
		if err := store.set(updated); err != nil {
			return err
		}
		fmt.Fprintf(out, "added to the front of the %s PATH\n", store.scope)
		fmt.Fprint(out, "open a new terminal for it to take effect\n")
	}

	reportShadowingLimits(dir, effective, store.scope, out)
	return nil
}

// reportShadowingLimits names the directories a wrapper in dir cannot shadow,
// so that a tool which stubbornly stays unwrapped is explained here rather
// than discovered later.
func reportShadowingLimits(dir string, effective []string, scope string, out io.Writer) {
	ahead := entriesAheadOf(effective, dir)
	if len(ahead) == 0 {
		fmt.Fprint(out, "nothing on PATH precedes it: any tool can be wrapped\n")
		return
	}
	fmt.Fprintf(out, "\n%d director%s on PATH still precede it, so tools in them\n",
		len(ahead), plural(len(ahead), "y", "ies"))
	fmt.Fprint(out, "cannot be wrapped from the wrapper folder:\n")
	for _, entry := range ahead {
		fmt.Fprintf(out, "  %s\n", entry)
	}
	if scope == "user" {
		fmt.Fprint(out, "\nWindows puts every machine PATH entry ahead of every user one, so\n")
		fmt.Fprint(out, "prepending to the user PATH cannot get past these. Use --setup --machine\n")
		fmt.Fprint(out, "(which needs administrator rights), or --adapt for those tools.\n")
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// effectivePathEntries is the PATH this process sees, which is what a tool
// spawned from here would resolve against.
func effectivePathEntries() []string {
	return pathEntries(os.Getenv("PATH"))
}

// runSetup is the --setup command.
func runSetup(out io.Writer, machine bool) error {
	dir, err := wrapperFolder()
	if err != nil {
		return err
	}
	store, err := persistentPathStore(machine)
	if err != nil {
		return err
	}
	return setupWrapperFolder(dir, store, effectivePathEntries(), out)
}

// joinPathList renders entries as a PATH value.
func joinPathList(entries []string) string {
	return strings.Join(entries, string(os.PathListSeparator))
}
