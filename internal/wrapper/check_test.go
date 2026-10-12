package wrapper

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckWrappers(t *testing.T) {
	t.Run("reports a healthy wrapper and its target", func(t *testing.T) {
		root := t.TempDir()
		wrapperDir := filepath.Join(root, "wrapper")
		toolDir := filepath.Join(root, "bin")
		writeFile(t, filepath.Join(wrapperDir, executableName("yt-dlp")), "wrapper "+wrapperMarker)
		writeFile(t, filepath.Join(toolDir, executableName("yt-dlp")), "the real tool")

		var out strings.Builder
		problems, err := checkWrappers(checkParams{
			wrapperDir:  wrapperDir,
			pathEntries: []string{wrapperDir, toolDir},
			probe:       realPathProbe(),
			out:         &out,
		})
		if err != nil {
			t.Fatalf("checkWrappers: %v", err)
		}
		if problems != 0 {
			t.Errorf("problems = %d, want 0\n%s", problems, out.String())
		}
		got := out.String()
		for _, want := range []string{"yt-dlp", "built-in defaults", filepath.Join(toolDir, executableName("yt-dlp"))} {
			if !strings.Contains(got, want) {
				t.Errorf("output does not mention %q:\n%s", want, got)
			}
		}
	})

	t.Run("flags a wrapper whose tool has gone", func(t *testing.T) {
		root := t.TempDir()
		wrapperDir := filepath.Join(root, "wrapper")
		writeFile(t, filepath.Join(wrapperDir, executableName("yt-dlp")), "wrapper "+wrapperMarker)

		var out strings.Builder
		problems, err := checkWrappers(checkParams{
			wrapperDir:  wrapperDir,
			pathEntries: []string{wrapperDir},
			probe:       realPathProbe(),
			out:         &out,
		})
		if err != nil {
			t.Fatalf("checkWrappers: %v", err)
		}
		if problems != 1 {
			t.Errorf("problems = %d, want 1", problems)
		}
		if !strings.Contains(out.String(), "UNRESOLVED") {
			t.Errorf("output = %q, want the unresolved target flagged", out.String())
		}
	})

	t.Run("flags a folder that is not on PATH", func(t *testing.T) {
		root := t.TempDir()
		wrapperDir := filepath.Join(root, "wrapper")
		toolDir := filepath.Join(root, "bin")
		writeFile(t, filepath.Join(wrapperDir, executableName("yt-dlp")), "wrapper "+wrapperMarker)
		writeFile(t, filepath.Join(toolDir, executableName("yt-dlp")), "the real tool")

		var out strings.Builder
		problems, err := checkWrappers(checkParams{
			wrapperDir:  wrapperDir,
			pathEntries: []string{toolDir},
			probe:       realPathProbe(),
			out:         &out,
		})
		if err != nil {
			t.Fatalf("checkWrappers: %v", err)
		}
		if problems == 0 {
			t.Error("problems = 0, want the missing PATH entry flagged")
		}
		if !strings.Contains(out.String(), "NOT on PATH") {
			t.Errorf("output = %q, want the PATH problem named", out.String())
		}
	})

	t.Run("flags a broken config", func(t *testing.T) {
		root := t.TempDir()
		wrapperDir := filepath.Join(root, "wrapper")
		writeFile(t, filepath.Join(wrapperDir, executableName("yt-dlp")), "wrapper "+wrapperMarker)
		writeFile(t, filepath.Join(wrapperDir, "yt-dlp.yaml"), "target: [broken\n")

		var out strings.Builder
		problems, err := checkWrappers(checkParams{
			wrapperDir:  wrapperDir,
			pathEntries: []string{wrapperDir},
			probe:       realPathProbe(),
			out:         &out,
		})
		if err != nil {
			t.Fatalf("checkWrappers: %v", err)
		}
		if problems != 1 {
			t.Errorf("problems = %d, want 1", problems)
		}
		if !strings.Contains(out.String(), "BROKEN") {
			t.Errorf("output = %q, want the broken config named", out.String())
		}
	})

	t.Run("says so when nothing is installed", func(t *testing.T) {
		root := t.TempDir()
		wrapperDir := filepath.Join(root, "wrapper")
		writeFile(t, filepath.Join(wrapperDir, "notes.txt"), "not a wrapper")

		var out strings.Builder
		if _, err := checkWrappers(checkParams{
			wrapperDir:  wrapperDir,
			pathEntries: []string{wrapperDir},
			probe:       realPathProbe(),
			out:         &out,
		}); err != nil {
			t.Fatalf("checkWrappers: %v", err)
		}
		if !strings.Contains(out.String(), "no wrappers installed") {
			t.Errorf("output = %q, want it to say the folder is empty of wrappers", out.String())
		}
	})

	t.Run("tells the user to run setup when the folder is absent", func(t *testing.T) {
		var out strings.Builder
		problems, err := checkWrappers(checkParams{
			wrapperDir:  filepath.Join(t.TempDir(), "never-made"),
			pathEntries: nil,
			probe:       realPathProbe(),
			out:         &out,
		})
		if err != nil {
			t.Fatalf("checkWrappers: %v", err)
		}
		if problems != 1 {
			t.Errorf("problems = %d, want 1", problems)
		}
		if !strings.Contains(out.String(), "--setup") {
			t.Errorf("output = %q, want it to point at --setup", out.String())
		}
	})
}
