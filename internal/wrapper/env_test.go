package wrapper

import (
	"runtime"
	"strings"
	"testing"
)

func lookupFrom(env []string) func(string) string {
	return func(name string) string {
		for _, entry := range env {
			if n, v, ok := splitEntry(entry); ok && envKey(n) == envKey(name) {
				return v
			}
		}
		return ""
	}
}

func valueOf(t *testing.T, env []string, name string) (string, bool) {
	t.Helper()
	var value string
	found := false
	for _, entry := range env {
		if n, v, ok := splitEntry(entry); ok && envKey(n) == envKey(name) {
			if found {
				t.Fatalf("%s appears more than once in %q", name, env)
			}
			value, found = v, true
		}
	}
	return value, found
}

func TestBuildEnvOverridesAndInherits(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HTTP_PROXY=http://old:1", "HOME=/home/u"}
	got, err := BuildEnv(base, []EnvVar{
		{"HTTP_PROXY", "http://proxy.example:8080"},
		{"NO_PROXY", "localhost,127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	if v, _ := valueOf(t, got, "HTTP_PROXY"); v != "http://proxy.example:8080" {
		t.Fatalf("HTTP_PROXY = %q, want the override", v)
	}
	if v, _ := valueOf(t, got, "NO_PROXY"); v != "localhost,127.0.0.1" {
		t.Fatalf("NO_PROXY = %q, want the new value", v)
	}
	for _, name := range []string{"PATH", "HOME"} {
		if _, ok := valueOf(t, got, name); !ok {
			t.Fatalf("%s was dropped; unmentioned variables must be inherited", name)
		}
	}
	if len(got) != len(base)+1 {
		t.Fatalf("len(env) = %d, want %d", len(got), len(base)+1)
	}
}

func TestBuildEnvDoesNotMutateBase(t *testing.T) {
	base := []string{"A=1"}
	if _, err := BuildEnv(base, []EnvVar{{"A", "2"}, {"B", "3"}}); err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	if base[0] != "A=1" || len(base) != 1 {
		t.Fatalf("base was modified: %q", base)
	}
}

func TestBuildEnvExpandsFromInheritedAndEarlierEntries(t *testing.T) {
	got, err := BuildEnv([]string{"PATH=/usr/bin", "HOST=proxy.example"}, []EnvVar{
		{"HTTP_PROXY", "http://${HOST}:8080"},
		{"HTTPS_PROXY", "${HTTP_PROXY}"},
		{"PATH", "/opt/tools:${PATH}"},
		{"LITERAL", "cost $$5 and $${HOST}"},
		{"UNSET", "[${NOPE}]"},
	})
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	want := map[string]string{
		"HTTP_PROXY":  "http://proxy.example:8080",
		"HTTPS_PROXY": "http://proxy.example:8080",
		"PATH":        "/opt/tools:/usr/bin",
		"LITERAL":     "cost $5 and ${HOST}",
		"UNSET":       "[]",
	}
	for name, wantValue := range want {
		if v, ok := valueOf(t, got, name); !ok || v != wantValue {
			t.Fatalf("%s = %q (present=%v), want %q", name, v, ok, wantValue)
		}
	}
}

func TestBuildEnvRejectsBrokenExpansion(t *testing.T) {
	for _, value := range []string{"${UNCLOSED", "${}", "${   }"} {
		if _, err := BuildEnv(nil, []EnvVar{{"A", value}}); err == nil {
			t.Fatalf("BuildEnv accepted %q", value)
		} else if !strings.Contains(err.Error(), "env A") {
			t.Fatalf("error should name the variable, got %v", err)
		}
	}
}

func TestBuildEnvKeepsWindowsDriveEntries(t *testing.T) {
	base := []string{`=C:=C:\work`, "A=1"}
	got, err := BuildEnv(base, []EnvVar{{"A", "2"}})
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	if got[0] != `=C:=C:\work` {
		t.Fatalf("env[0] = %q, want the drive entry untouched", got[0])
	}
}

func TestBuildEnvOverrideIsCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("environment names are case-sensitive off Windows")
	}
	got, err := BuildEnv([]string{"http_proxy=http://old:1"}, []EnvVar{
		{"HTTP_PROXY", "http://new:2"},
	})
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("env = %q, want a single replaced entry", got)
	}
	if v, _ := valueOf(t, got, "HTTP_PROXY"); v != "http://new:2" {
		t.Fatalf("HTTP_PROXY = %q, want the override", v)
	}
}

func TestExpandPassesThroughPlainValues(t *testing.T) {
	lookup := lookupFrom([]string{"A=1"})
	for _, s := range []string{"", "plain", `C:\tools\yt.exe`, "a$b", "trailing$", "100$"} {
		got, err := Expand(s, lookup)
		if err != nil {
			t.Fatalf("Expand(%q): %v", s, err)
		}
		if got != s {
			t.Fatalf("Expand(%q) = %q, want it unchanged", s, got)
		}
	}
}
