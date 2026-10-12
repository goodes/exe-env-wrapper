package wrapper

import (
	"path/filepath"
	"strings"
	"testing"
)

// The marker is found wherever it sits, including straddling the boundary
// between two reads. A binary is scanned in chunks, so a marker split across
// two of them must still be found.
func TestReaderContainsMarkerAcrossChunkBoundary(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		chunk int
		want  bool
	}{
		{name: "absent", body: strings.Repeat("x", 500), chunk: 64, want: false},
		{name: "at the very start", body: wrapperMarker + strings.Repeat("x", 100), chunk: 64, want: true},
		{name: "at the very end", body: strings.Repeat("x", 100) + wrapperMarker, chunk: 64, want: true},
		{
			name:  "straddling a chunk boundary",
			body:  strings.Repeat("x", 60) + wrapperMarker + strings.Repeat("x", 60),
			chunk: 64,
			want:  true,
		},
		{
			name:  "chunk smaller than the marker",
			body:  strings.Repeat("x", 10) + wrapperMarker,
			chunk: 8,
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readerContainsMarker(strings.NewReader(tt.body), tt.chunk)
			if err != nil {
				t.Fatalf("readerContainsMarker: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// The marker has to survive into a real build, which is the whole point of
// it. The linker is free to drop a string no reachable code references, so
// this asserts against an actual compiled wrapper rather than the constant.
func TestBuiltWrapperCarriesTheMarker(t *testing.T) {
	dir, wrapperPath := install(t, "yt", "target: yt-cli\n")

	if !isWrapperBinary(wrapperPath) {
		t.Errorf("a built wrapper at %s is not recognised as one", wrapperPath)
	}

	child := filepath.Join(dir, "yt-cli"+exeSuffix())
	if isWrapperBinary(child) {
		t.Errorf("the target binary at %s was mistaken for a wrapper", child)
	}
}
