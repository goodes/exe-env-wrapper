package wrapper

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type upgradeFixture struct {
	dir    string
	target string
	self   string
	out    strings.Builder
}

// newVersionForTest is the version stamped into the replacement binary. The
// test binary's own Version is "dev", which does not read back as a version at
// all, so from and to are both stamped explicitly.
const newVersionForTest = "v9.9.9-new"

// newUpgradeFixture builds an installed wrapper at an older version and a
// newer binary to replace it with.
func newUpgradeFixture(t *testing.T, oldVersion string) *upgradeFixture {
	t.Helper()
	root := t.TempDir()
	f := &upgradeFixture{
		dir:    root,
		target: filepath.Join(root, "bin", executableName("yt-dlp")),
		self:   filepath.Join(root, "new", executableName("exe-env-wrapper")),
	}
	if err := os.MkdirAll(filepath.Dir(f.target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(f.self), 0o755); err != nil {
		t.Fatal(err)
	}
	goBuildVersioned(t, repoRoot(t), "./cmd/exe-env-wrapper", f.target, oldVersion)
	goBuildVersioned(t, repoRoot(t), "./cmd/exe-env-wrapper", f.self, newVersionForTest)
	return f
}

func (f *upgradeFixture) params() upgradeParams {
	return upgradeParams{target: f.target, self: f.self, out: &f.out}
}

// The ordinary case: an older wrapper is replaced, and the output says what it
// was and what it became.
func TestUpgradeReplacesAnOlderWrapper(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")
	before, err := os.ReadFile(f.target)
	if err != nil {
		t.Fatal(err)
	}

	if err := upgradeWrapper(f.params()); err != nil {
		t.Fatalf("upgradeWrapper: %v", err)
	}

	after, err := os.ReadFile(f.target)
	if err != nil {
		t.Fatalf("reading the replaced wrapper: %v", err)
	}
	if string(before) == string(after) {
		t.Error("the wrapper was not replaced")
	}
	if !isWrapperBinary(f.target) {
		t.Error("what was installed is not a wrapper")
	}

	got := f.out.String()
	for _, want := range []string{"v0.1.0-old", newVersionForTest, "replaced"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}

	// The moved-aside file is cleaned up when nothing holds it open.
	if _, err := os.Stat(f.target + ".old"); !os.IsNotExist(err) {
		t.Errorf("%s.old was left behind although nothing was using it", f.target)
	}
}

// The check that makes naming a path safe. Pointed at the wrong file, upgrade
// has to refuse rather than overwrite it.
func TestUpgradeRefusesSomethingThatIsNotAWrapper(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")
	victim := filepath.Join(f.dir, executableName("important-tool"))
	buildChildAs(t, victim)
	before, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}

	p := f.params()
	p.target = victim
	err = upgradeWrapper(p)
	if err == nil {
		t.Fatal("upgradeWrapper replaced a binary that is not a wrapper")
	}
	if !strings.Contains(err.Error(), "not an exe-env-wrapper") {
		t.Errorf("err = %v, want a refusal naming the cause", err)
	}

	after, _ := os.ReadFile(victim)
	if string(before) != string(after) {
		t.Error("the refused file was modified anyway")
	}
}

func TestUpgradeRefusesAMissingFile(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")
	p := f.params()
	p.target = filepath.Join(f.dir, executableName("absent"))

	err := upgradeWrapper(p)
	if err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("err = %v, want a not-a-file error", err)
	}
}

// Replacing a wrapper with the same build is a no-op, so running upgrade over
// a folder's worth of wrappers costs nothing for the ones already current.
func TestUpgradeIsANoOpAtTheSameVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, newVersionForTest)
	before, err := os.ReadFile(f.target)
	if err != nil {
		t.Fatal(err)
	}

	if err := upgradeWrapper(f.params()); err != nil {
		t.Fatalf("upgradeWrapper: %v", err)
	}
	if !strings.Contains(f.out.String(), "nothing to do") {
		t.Errorf("output = %q, want it to say nothing was needed", f.out.String())
	}

	after, _ := os.ReadFile(f.target)
	if string(before) != string(after) {
		t.Error("the wrapper was replaced although it was already current")
	}
}

func TestUpgradeForceReplacesTheSameVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, newVersionForTest)
	p := f.params()
	p.force = true

	if err := upgradeWrapper(p); err != nil {
		t.Fatalf("upgradeWrapper: %v", err)
	}
	if !strings.Contains(f.out.String(), "replaced") {
		t.Errorf("output = %q, want it to have replaced under --force", f.out.String())
	}
}

// Everything installed before the version stamp existed reads as unknown. It
// is still a wrapper and still replaceable, or those installs could never be
// upgraded.
func TestUpgradeReplacesAnUnstampedWrapper(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")
	// A wrapper carrying the marker but no stamp, as v0.3.2 and earlier do.
	writeFile(t, f.target, "padding "+wrapperMarker+" padding")

	if err := upgradeWrapper(f.params()); err != nil {
		t.Fatalf("upgradeWrapper: %v", err)
	}
	if !strings.Contains(f.out.String(), "unknown") {
		t.Errorf("output = %q, want the unknown version reported", f.out.String())
	}
	if !isWrapperBinary(f.target) {
		t.Error("the unstamped wrapper was not replaced with a real one")
	}
}

// A windowless wrapper must not be replaced with the console build: that would
// reintroduce the window flash under a host with no console, silently.
//
// Built for real rather than faked with a synthetic stamp. The first version
// of this test wrote the marker and a stamp into a file by hand and passed
// while the real thing was broken: a genuine windowless build carried no stamp
// at all, because the linker dropped the string along with the command mode
// that read it.
func TestUpgradeRefusesToChangeVariantWithoutTheWindowlessBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	if executableName("x") != "x.exe" {
		t.Skip("the two builds differ only on Windows")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")
	goBuildWindowless(t, f.target, "v0.1.0-old")

	if variant, ok := readVariantFromFile(f.target); !ok || variant != variantWindowless {
		t.Fatalf("a real windowless build reads as %q (known=%v), want %q",
			variant, ok, variantWindowless)
	}

	err := upgradeWrapper(f.params())
	if err == nil {
		t.Fatal("upgradeWrapper replaced a windowless wrapper with the console build")
	}
	if !strings.Contains(err.Error(), variantWindowless) {
		t.Errorf("err = %v, want it to name the variant mismatch", err)
	}
}

// With a real windowless build beside it, the variant is preserved.
func TestUpgradeKeepsTheWindowlessVariant(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	if executableName("x") != "x.exe" {
		t.Skip("the two builds differ only on Windows")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")
	goBuildWindowless(t, f.target, "v0.1.0-old")

	// The -w build beside the console one, which is how the downloads arrive.
	sibling := filepath.Join(filepath.Dir(f.self), "exe-env-wrapper-w-windows-amd64.exe")
	goBuildWindowless(t, sibling, newVersionForTest)

	if err := upgradeWrapper(f.params()); err != nil {
		t.Fatalf("upgradeWrapper: %v\n%s", err, f.out.String())
	}

	// The installed wrapper must still be the windowless build, and must be
	// the new version.
	if variant, ok := readVariantFromFile(f.target); !ok || variant != variantWindowless {
		t.Errorf("installed variant = %q (known=%v), want %q", variant, ok, variantWindowless)
	}
	if version, ok := readVersionFromFile(f.target); !ok || version != newVersionForTest {
		t.Errorf("installed version = %q (known=%v), want %q", version, ok, newVersionForTest)
	}
}

// Windows will not overwrite a running executable, and replacing a wrapper
// while the tool it wraps is mid-run is a realistic thing to be doing. The
// rename-aside exists for this.
func TestUpgradeReplacesAWrapperThatIsRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	f := newUpgradeFixture(t, "v0.1.0-old")

	// A target that waits on stdin, so the wrapper stays alive while held.
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(stdioChildSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(childModule), 0o644); err != nil {
		t.Fatal(err)
	}
	goBuild(t, src, ".", filepath.Join(filepath.Dir(f.target), executableName("yt-dlp-cli")))
	write(t, filepath.Join(filepath.Dir(f.target), "yt-dlp.yaml"),
		"target: yt-dlp-cli"+exeSuffix()+"\n")

	cmd := exec.Command(f.target)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the wrapper: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	// Upgrade it out from under the running process.
	if err := upgradeWrapper(f.params()); err != nil {
		t.Fatalf("upgradeWrapper on a running wrapper: %v\n%s", err, f.out.String())
	}

	if !strings.Contains(f.out.String(), "replaced") {
		t.Errorf("output = %q, want the replacement to have happened", f.out.String())
	}
	if version, ok := readVersionFromFile(f.target); !ok || version != newVersionForTest {
		t.Errorf("installed version = %q (known=%v), want %q", version, ok, newVersionForTest)
	}
	// The old file cannot be deleted while the process holds it, so it is
	// reported rather than silently left.
	if _, err := os.Stat(f.target + ".old"); err == nil {
		if !strings.Contains(f.out.String(), "still in use") {
			t.Errorf("output = %q, want it to name the file left behind", f.out.String())
		}
	}
}
