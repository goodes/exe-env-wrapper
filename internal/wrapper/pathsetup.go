package wrapper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// wrapperFolderName is where wrappers are installed, relative to the user's
// home. A fixed location keeps --wrap, --check and the PATH advice talking
// about the same directory.
var wrapperFolderName = filepath.Join(".local", "wrapper")

// wrapperFolder is the absolute path of the wrapper folder.
func wrapperFolder() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory: %w", err)
	}
	return filepath.Join(home, wrapperFolderName), nil
}

// filepathToSlash is filepath.ToSlash, wrapped so tests can talk about paths
// without caring which separator the platform uses.
func filepathToSlash(p string) string {
	return filepath.ToSlash(p)
}

// samePathEntry reports whether two PATH entries name the same directory.
//
// Compared as text rather than by identity, because a PATH list routinely
// holds directories that do not exist, and --setup still has to recognise its
// own entry among them. Case is ignored on Windows, where file names are.
func samePathEntry(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

// prependDir returns entries with dir first and any later copy of it removed,
// along with whether that differs from what was passed in.
func prependDir(entries []string, dir string) ([]string, bool) {
	// dir is written through verbatim. It arrives already in native form, and
	// re-normalising it would put a different spelling of the same directory
	// into the user's PATH than the one reported to them.
	out := make([]string, 0, len(entries)+1)
	out = append(out, dir)
	for _, entry := range entries {
		if entry == "" || samePathEntry(entry, dir) {
			continue
		}
		out = append(out, entry)
	}

	changed := len(out) != len(entries)
	if !changed {
		for i := range out {
			if out[i] != entries[i] {
				changed = true
				break
			}
		}
	}
	return out, changed
}

// entriesAheadOf returns the PATH entries that precede dir, which are the
// directories a wrapper in dir cannot shadow. A directory absent from PATH has
// everything ahead of it.
func entriesAheadOf(effective []string, dir string) []string {
	for i, entry := range effective {
		if samePathEntry(entry, dir) {
			return effective[:i]
		}
	}
	return effective
}
