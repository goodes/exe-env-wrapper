package wrapper

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPrependDir(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		dir     string
		want    []string
		changed bool
	}{
		{
			name:    "absent, so it goes first",
			entries: []string{"/a", "/b"},
			dir:     "/w",
			want:    []string{"/w", "/a", "/b"},
			changed: true,
		},
		{
			name:    "already first, so nothing changes",
			entries: []string{"/w", "/a"},
			dir:     "/w",
			want:    []string{"/w", "/a"},
			changed: false,
		},
		{
			name:    "present but too late, so it moves to the front",
			entries: []string{"/a", "/w", "/b"},
			dir:     "/w",
			want:    []string{"/w", "/a", "/b"},
			changed: true,
		},
		{
			name:    "an empty list",
			entries: nil,
			dir:     "/w",
			want:    []string{"/w"},
			changed: true,
		},
		{
			name:    "a duplicate later on is removed",
			entries: []string{"/w", "/a", "/w"},
			dir:     "/w",
			want:    []string{"/w", "/a"},
			changed: true,
		},
		{
			name:    "a different spelling counts as the same entry",
			entries: []string{"/a", "/w/"},
			dir:     "/w",
			want:    []string{"/w", "/a"},
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := prependDir(tt.entries, tt.dir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entries = %v, want %v", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
		})
	}
}

// Windows composes the effective PATH as every machine entry followed by every
// user entry, so a user entry can never precede a machine one. Anything ahead
// of the wrapper folder cannot be shadowed from there, and --setup has to say
// so rather than leave the user to discover it.
func TestEntriesAheadOf(t *testing.T) {
	effective := []string{"/sys/one", "/sys/two", "/w", "/user/one"}

	got := entriesAheadOf(effective, "/w")
	want := []string{"/sys/one", "/sys/two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if got := entriesAheadOf(effective, "/absent"); len(got) != len(effective) {
		t.Errorf("a directory not on PATH has everything ahead of it, got %v", got)
	}

	if got := entriesAheadOf([]string{"/w", "/a"}, "/w"); len(got) != 0 {
		t.Errorf("first entry has nothing ahead of it, got %v", got)
	}
}

func TestSamePathEntry(t *testing.T) {
	same := [][2]string{
		{"/a/b", "/a/b"},
		{"/a/b/", "/a/b"},
		{"/a/./b", "/a/b"},
	}
	for _, pair := range same {
		if !samePathEntry(pair[0], pair[1]) {
			t.Errorf("samePathEntry(%q, %q) = false, want true", pair[0], pair[1])
		}
	}
	if samePathEntry("/a/b", "/a/c") {
		t.Error("samePathEntry matched two different directories")
	}
}

// The folder is a fixed location under the user's home, named in the help and
// the README, so a test pins it rather than letting it drift.
func TestWrapperFolderIsUnderHome(t *testing.T) {
	got, err := wrapperFolder()
	if err != nil {
		t.Fatalf("wrapperFolder: %v", err)
	}
	if !strings.HasSuffix(filepathToSlash(got), ".local/wrapper") {
		t.Errorf("wrapperFolder() = %q, want it to end in .local/wrapper", got)
	}
}

// fakePathStore records what setup would have written.
type fakePathStore struct {
	entries []string
	written [][]string
	failSet error
}

func (f *fakePathStore) store(scope string) pathStore {
	return pathStore{
		scope: scope,
		get:   func() ([]string, error) { return f.entries, nil },
		set: func(e []string) error {
			if f.failSet != nil {
				return f.failSet
			}
			f.written = append(f.written, e)
			return nil
		},
	}
}

func TestSetupWrapperFolder(t *testing.T) {
	t.Run("creates the folder and claims the front of PATH", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "wrapper")
		fake := &fakePathStore{entries: []string{"/a", "/b"}}
		var out strings.Builder

		if err := setupWrapperFolder(dir, fake.store("user"), []string{"/a", "/b"}, &out); err != nil {
			t.Fatalf("setupWrapperFolder: %v", err)
		}

		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Errorf("wrapper folder was not created at %s", dir)
		}
		if len(fake.written) != 1 {
			t.Fatalf("wrote PATH %d times, want 1", len(fake.written))
		}
		if fake.written[0][0] != dir {
			t.Errorf("PATH starts with %q, want %q", fake.written[0][0], dir)
		}
		if !strings.Contains(out.String(), "added to the front of the user PATH") {
			t.Errorf("output = %q, want it to say what it did", out.String())
		}
	})

	t.Run("is idempotent when already leading", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "wrapper")
		fake := &fakePathStore{entries: []string{dir, "/a"}}
		var out strings.Builder

		if err := setupWrapperFolder(dir, fake.store("user"), []string{dir, "/a"}, &out); err != nil {
			t.Fatalf("setupWrapperFolder: %v", err)
		}

		if len(fake.written) != 0 {
			t.Errorf("rewrote PATH when it was already correct: %v", fake.written)
		}
		if !strings.Contains(out.String(), "already first") {
			t.Errorf("output = %q, want it to say nothing needed doing", out.String())
		}
	})

	t.Run("names what it cannot shadow", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "wrapper")
		fake := &fakePathStore{entries: []string{"/user/one"}}
		var out strings.Builder

		// The wrapper folder sits behind two machine entries, which prepending
		// to the user list cannot fix.
		effective := []string{"/machine/one", "/machine/two", dir, "/user/one"}
		if err := setupWrapperFolder(dir, fake.store("user"), effective, &out); err != nil {
			t.Fatalf("setupWrapperFolder: %v", err)
		}

		got := out.String()
		for _, want := range []string{"/machine/one", "/machine/two", "--setup --machine", "--adapt"} {
			if !strings.Contains(got, want) {
				t.Errorf("output does not mention %q:\n%s", want, got)
			}
		}
	})
}
