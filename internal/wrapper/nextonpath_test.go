package wrapper

import (
	"path/filepath"
	"strings"
	"testing"
)

// fakeProbe answers resolution's filesystem questions from plain data, so the
// ordering rules can be tested without laying out directories.
type fakeProbe struct {
	// files are the paths that exist, already in executableName form.
	files map[string]bool
	// wrappers are the subset of those that are exe-env-wrapper builds.
	wrappers map[string]bool
	// aliases maps a PATH spelling to the canonical directory it denotes, so
	// two spellings of one directory compare equal.
	aliases map[string]string
}

func (f fakeProbe) probe() pathProbe {
	canon := func(dir string) string {
		if c, ok := f.aliases[dir]; ok {
			return c
		}
		return dir
	}
	return pathProbe{
		sameDir:   func(a, b string) bool { return canon(a) == canon(b) },
		isFile:    func(p string) bool { return f.files[p] },
		isWrapper: func(p string) bool { return f.wrappers[p] },
	}
}

func exe(dir, base string) string {
	return filepath.Join(dir, executableName(base))
}

func TestResolveNextOnPath(t *testing.T) {
	const own = "/opt/wrapper"

	tests := []struct {
		name    string
		entries []string
		probe   fakeProbe
		want    string
	}{
		{
			name:    "takes the first entry that is not the wrapper's own directory",
			entries: []string{own, "/usr/local/bin", "/usr/bin"},
			probe: fakeProbe{files: map[string]bool{
				exe(own, "yt-dlp"):              true,
				exe("/usr/local/bin", "yt-dlp"): true,
				exe("/usr/bin", "yt-dlp"):       true,
			}},
			want: exe("/usr/local/bin", "yt-dlp"),
		},
		{
			name:    "skips the own directory under a different spelling",
			entries: []string{"/opt/WRAPPER/.", "/usr/bin"},
			probe: fakeProbe{
				files: map[string]bool{
					exe("/opt/WRAPPER/.", "yt-dlp"): true,
					exe("/usr/bin", "yt-dlp"):       true,
				},
				aliases: map[string]string{"/opt/WRAPPER/.": own},
			},
			want: exe("/usr/bin", "yt-dlp"),
		},
		{
			name:    "skips directories where the name is absent",
			entries: []string{"/empty", "/usr/bin"},
			probe: fakeProbe{files: map[string]bool{
				exe("/usr/bin", "yt-dlp"): true,
			}},
			want: exe("/usr/bin", "yt-dlp"),
		},
		{
			name:    "skips another wrapper rather than handing off to it",
			entries: []string{"/other/wrapper", "/usr/bin"},
			probe: fakeProbe{
				files: map[string]bool{
					exe("/other/wrapper", "yt-dlp"): true,
					exe("/usr/bin", "yt-dlp"):       true,
				},
				wrappers: map[string]bool{exe("/other/wrapper", "yt-dlp"): true},
			},
			want: exe("/usr/bin", "yt-dlp"),
		},
		{
			name:    "ignores empty PATH entries",
			entries: []string{"", "/usr/bin"},
			probe: fakeProbe{files: map[string]bool{
				exe("/usr/bin", "yt-dlp"): true,
			}},
			want: exe("/usr/bin", "yt-dlp"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveNextOnPath("yt-dlp", own, tt.entries, tt.probe.probe())
			if err != nil {
				t.Fatalf("resolveNextOnPath: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// With nothing to hand off to, the error has to say what was looked for and
// where, because the config that led here names no target at all.
func TestResolveNextOnPathReportsWhatItSearched(t *testing.T) {
	probe := fakeProbe{}.probe()

	_, err := resolveNextOnPath("yt-dlp", "/opt/wrapper", []string{"/opt/wrapper", "/usr/bin"}, probe)
	if err == nil {
		t.Fatal("resolveNextOnPath found something in an empty filesystem")
	}
	if !strings.Contains(err.Error(), "yt-dlp") {
		t.Errorf("error %q should name what it looked for", err)
	}
	if !strings.Contains(err.Error(), "/usr/bin") {
		t.Errorf("error %q should name the directories it searched", err)
	}
}
