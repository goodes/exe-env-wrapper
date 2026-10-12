package wrapper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// hashLength is how much of the digest goes into the renamed file's name. The
// hash is a content tag rather than a security boundary, so it only has to be
// long enough to distinguish one build of a tool from the next, and short
// enough to leave the name readable: yt-dlp-3f9a1c2b.exe.
const hashLength = 8

// adaptParams is everything --adapt needs.
type adaptParams struct {
	// file is the binary to adapt: a path, or a bare name in ownDir.
	file string
	// ownDir is the wrapper's own directory, where a bare name is looked for.
	ownDir string
	// sourceExe is the wrapper binary to install under the original name.
	sourceExe string
	// force overwrites an existing config.
	force bool
	out   io.Writer
}

// adaptBinary replaces a binary in place: the original is renamed aside with a
// tag of its contents, and the wrapper takes its name.
//
// This is for callers that hard-code a path, where PATH is never consulted and
// so the wrapper folder cannot help. It is the weaker option everywhere else:
// an installer that regenerates the binary, as uv tool upgrade does, will
// overwrite the wrapper and silently undo it.
func adaptBinary(p adaptParams) error {
	path, err := resolveAdaptTarget(p)
	if err != nil {
		return err
	}
	if err := checkWrappable(path); err != nil {
		return err
	}

	digest, err := fileDigest(path)
	if err != nil {
		return err
	}
	aside := hashedName(path, digest)

	if err := moveAside(path, aside, p.out); err != nil {
		return err
	}
	if err := copyExecutable(p.sourceExe, path); err != nil {
		return err
	}
	fmt.Fprintf(p.out, "installed %s\n", path)

	return writeAdaptConfig(p, path, aside)
}

// resolveAdaptTarget locates the binary to adapt. Unlike --wrap, PATH is never
// consulted: a name resolved from PATH would be the wrong file to replace,
// since the whole reason to adapt is that the caller is not using PATH.
func resolveAdaptTarget(p adaptParams) (string, error) {
	path := p.file
	if !isPathArgument(path) {
		path = filepath.Join(p.ownDir, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", p.file, err)
	}
	if !isRegularFile(path) {
		return "", fmt.Errorf("not a file: %s", path)
	}
	return path, nil
}

// fileDigest is the truncated SHA-256 of a file's contents.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil))[:hashLength], nil
}

// hashedName is where the original binary is moved to: its own name with the
// content tag inserted before the extension.
func hashedName(path, digest string) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(filepath.Base(path), ext)
	return filepath.Join(filepath.Dir(path), stem+"-"+digest+ext)
}

// moveAside renames the original, unless an identical copy is already there
// from a previous run.
func moveAside(path, aside string, out io.Writer) error {
	if isRegularFile(aside) {
		// The name carries a hash of the contents, so a file already at that
		// name with that hash is this same binary from an earlier --adapt.
		// Re-running is then a no-op rather than an error.
		existing, err := fileDigest(aside)
		current, err2 := fileDigest(path)
		if err == nil && err2 == nil && existing == current {
			fmt.Fprintf(out, "already aside %s\n", aside)
			return nil
		}
		return fmt.Errorf("%s already exists with different contents; move it away first", aside)
	}
	if err := os.Rename(path, aside); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", path, aside, err)
	}
	fmt.Fprintf(out, "moved     %s\n", aside)
	return nil
}

// writeAdaptConfig records the renamed binary as the target. Unlike a wrapped
// tool, an adapted one cannot be resolved from PATH, so the config is not
// optional here.
func writeAdaptConfig(p adaptParams, path, aside string) error {
	configPath := filepath.Join(filepath.Dir(path), BaseName(path)+".yaml")
	if _, err := os.Stat(configPath); err == nil && !p.force {
		// Someone's edited file. Losing it to a re-run would be worse than
		// refusing, and the target line is the only part that must be right.
		fmt.Fprintf(p.out, "config    %s left alone (already exists; --force to replace)\n", configPath)
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "target: %s\n", filepath.Base(aside))
	b.WriteString("env:\n")
	b.WriteString("  HTTP_PROXY: ${system.http_proxy}\n")
	b.WriteString("  HTTPS_PROXY: ${system.https_proxy}\n")
	b.WriteString("  NO_PROXY: ${system.no_proxy}\n")

	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}
	fmt.Fprintf(p.out, "config    %s\n", configPath)
	return nil
}

// runAdapt is the --adapt command.
func runAdapt(out io.Writer, args []string) error {
	file := firstOperand(args)
	if file == "" {
		return fmt.Errorf("--adapt needs a path to a binary")
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine own path: %w", err)
	}
	source, windowless := wrapperSourceExe(self, hasFlag(args, "--console"))
	if windowless {
		fmt.Fprintf(out, "using     %s (windowless)\n", filepath.Base(source))
	}
	return adaptBinary(adaptParams{
		file:      file,
		ownDir:    filepath.Dir(self),
		sourceExe: source,
		force:     hasFlag(args, "--force"),
		out:       out,
	})
}
