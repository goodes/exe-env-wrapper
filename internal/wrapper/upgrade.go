package wrapper

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// upgradeParams is everything upgrade needs.
type upgradeParams struct {
	// target is the installed wrapper to replace, named explicitly.
	target string
	// self is this binary, the replacement.
	self string
	// force replaces a wrapper already at this version.
	force bool
	out   io.Writer
}

// upgradeWrapper replaces one installed wrapper with this binary.
//
// One at a time, named explicitly. There is no enumeration and no record of
// where wrappers were installed: a stored list goes stale the moment a file is
// moved by hand, and scanning PATH would mean marker-scanning every binary on
// the machine. What makes explicit naming safe is that the file is checked
// before it is touched, so pointing this at the wrong thing refuses rather
// than destroys.
func upgradeWrapper(p upgradeParams) error {
	path, err := filepath.Abs(p.target)
	if err != nil {
		return fmt.Errorf("%s: %w", p.target, err)
	}
	if !isRegularFile(path) {
		return fmt.Errorf("not a file: %s", path)
	}
	// The check that makes this safe. Without it, upgrade pointed at the wrong
	// path would overwrite whatever was there with a wrapper.
	if !isWrapperBinary(path) {
		return fmt.Errorf("%s is not an exe-env-wrapper; refusing to replace it", path)
	}

	source, err := upgradeSource(p.self, path)
	if err != nil {
		return err
	}

	oldVersion, oldKnown := readVersionFromFile(path)
	if !oldKnown {
		// Everything installed before the version stamp existed reads this
		// way. It is still a wrapper, and still replaceable.
		oldVersion = "unknown"
	}

	// Read from the file being installed rather than from this binary's own
	// Version: when the source is the windowless sibling, that is a different
	// file and could be a different build.
	newVersion, newKnown := readVersionFromFile(source)
	if !newKnown {
		newVersion = Version
	}

	fmt.Fprintf(p.out, "%s\n", path)
	fmt.Fprintf(p.out, "  from  %s\n", oldVersion)
	fmt.Fprintf(p.out, "  to    %s\n", newVersion)

	if oldKnown && newKnown && oldVersion == newVersion && !p.force {
		fmt.Fprintf(p.out, "  already at %s; nothing to do (--force to replace anyway)\n", newVersion)
		return nil
	}

	if err := replaceExecutable(source, path, p.out); err != nil {
		return err
	}
	fmt.Fprint(p.out, "  replaced\n")
	return nil
}

// upgradeSource picks the replacement binary, matching the variant already
// installed.
//
// Replacing a windowless wrapper with a console one would make Windows
// allocate a console under a host that has none, flashing a window on every
// run — a silent regression in exactly the setup the windowless build exists
// for.
func upgradeSource(self, target string) (string, error) {
	variant, known := readVariantFromFile(target)
	if !known || variant == buildVariant() {
		// An unstamped wrapper predates the variant stamp. This binary is the
		// console build, since the windowless one has no commands, and the
		// console build is the right default anyway.
		return self, nil
	}

	sibling, ok := findWindowlessSibling(self)
	if !ok {
		return "", fmt.Errorf(
			"%s is the %s build, and no %s-w-<platform>.exe sits beside %s to replace it with",
			target, variant, wrapperNamePrefix, filepath.Dir(self))
	}
	return sibling, nil
}

// replaceExecutable swaps dest for src, tolerating dest being in use.
//
// Windows refuses to overwrite or delete a running executable but allows it to
// be renamed, so the old file is moved aside and the new one written in its
// place. A process still running the old one carries on from the renamed file.
// Removing that file is attempted and forgiven, because it fails for exactly
// the reason the rename was needed.
func replaceExecutable(src, dest string, out io.Writer) error {
	aside := dest + ".old"
	// A leftover from a previous upgrade whose process has since exited.
	_ = os.Remove(aside)

	if err := os.Rename(dest, aside); err != nil {
		return fmt.Errorf("moving %s aside: %w", dest, err)
	}
	if err := copyExecutable(src, dest); err != nil {
		// Put it back rather than leave nothing where the tool used to be.
		if restoreErr := os.Rename(aside, dest); restoreErr != nil {
			return fmt.Errorf("writing %s failed (%w) and %s could not be restored: %v",
				dest, err, aside, restoreErr)
		}
		return err
	}
	if err := os.Remove(aside); err != nil {
		fmt.Fprintf(out, "  left %s behind: still in use, delete it later\n", filepath.Base(aside))
	}
	return nil
}

// runUpgrade is the upgrade command.
func runUpgrade(out io.Writer, target string, opts map[string]bool) error {
	if target == "" {
		return fmt.Errorf("upgrade needs the path of a wrapper to replace; try \"help upgrade\"")
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine own path: %w", err)
	}
	return upgradeWrapper(upgradeParams{
		target: target,
		self:   self,
		force:  opts["--force"],
		out:    out,
	})
}
