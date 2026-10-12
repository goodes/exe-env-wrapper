package wrapper

import "testing"

// The gate decides whether an argument belongs to the wrapper or to the tool
// it wraps. It is a prefix match rather than an exact name because the
// published downloads carry platform suffixes, and those are the names a
// first-time user actually invokes out of a downloads folder.
func TestHasWrapperName(t *testing.T) {
	tests := map[string]bool{
		"exe-env-wrapper":                 true,
		"exe-env-wrapper-w":               true,
		"exe-env-wrapper-windows-amd64":   true,
		"exe-env-wrapper-w-windows-amd64": true,
		"exe-env-wrapper-linux-arm64":     true,
		"EXE-ENV-WRAPPER":                 true,
		"yt":                              false,
		"yt-dlp":                          false,
		"uv":                              false,
		"claude":                          false,
		"my-exe-env-wrapper":              false,
		"exe-env":                         false,
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := hasWrapperName(name); got != want {
				t.Errorf("hasWrapperName(%q) = %v, want %v", name, got, want)
			}
		})
	}
}
