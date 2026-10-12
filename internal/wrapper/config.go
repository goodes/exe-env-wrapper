// Package wrapper implements the config-driven launcher behind exe-env-wrapper:
// it finds the YAML file matching its own executable name, applies the
// environment overrides from it, and hands off to the target binary.
package wrapper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config mirrors the YAML file that sits beside the wrapper executable.
type Config struct {
	// Target is the binary to run: absolute, or relative to the wrapper's directory.
	Target string
	// Env holds the variables to set or override, in the order the file lists them.
	Env []EnvVar
	// Log is an optional file that wrapper errors are appended to. The windowless
	// Windows build has no visible stderr, so it is often the only way to see them.
	Log string
}

// EnvVar is a single env entry. Keeping the file's order lets a later value
// refer to an earlier one through ${VAR}.
type EnvVar struct {
	Name  string
	Value string
	// OmitIfEmpty leaves the variable as inherited when the value expands to
	// nothing, rather than setting it empty. Only the built-in defaults use
	// it: on a machine with no proxy configured ${system.http_proxy} expands
	// to nothing, and a default that blanked a proxy the caller had set would
	// do active harm. A value written in a config file is always applied, so
	// setting a variable empty on purpose still works.
	OmitIfEmpty bool
}

// ErrNoConfig reports that no config file exists beside the wrapper.
//
// It is not necessarily a failure. A wrapper installed into the wrapper folder
// needs no config file: it runs on the built-in defaults and resolves its
// target from PATH, which is what makes installing one a single copy.
var ErrNoConfig = errors.New("no config file")

// DefaultConfig is what a wrapper with no config file applies: the system's
// proxy settings, and no target, so the target comes from PATH.
//
// The bypass list is the point. A client built on httpx picks up the proxy
// variables on its own but never the system's bypass list, so NO_PROXY is the
// one that has to be handed to it.
func DefaultConfig() *Config {
	return &Config{
		Env: []EnvVar{
			{Name: "HTTP_PROXY", Value: "${system.http_proxy}", OmitIfEmpty: true},
			{Name: "HTTPS_PROXY", Value: "${system.https_proxy}", OmitIfEmpty: true},
			{Name: "NO_PROXY", Value: "${system.no_proxy}", OmitIfEmpty: true},
		},
	}
}

// rawConfig decodes env as a node rather than a map so the ordering survives.
type rawConfig struct {
	Target string    `yaml:"target"`
	Env    yaml.Node `yaml:"env"`
	Log    string    `yaml:"log"`
}

// ParseConfig reads the YAML body of a wrapper config.
func ParseConfig(data []byte) (*Config, error) {
	var raw rawConfig
	// KnownFields rejects a key the struct has no field for, so a mistyped
	// "targett" is reported rather than silently ignored.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	cfg := &Config{
		Target: strings.TrimSpace(raw.Target),
		Log:    strings.TrimSpace(raw.Log),
	}
	// An absent target is not an error: it means "resolve the next match for my
	// own name on PATH". That is what lets a wrapper in the wrapper folder
	// shadow a tool whose real location it does not need to know, and survive
	// that tool being regenerated or moved. Having nowhere to hand off to is
	// reported at resolution time, where the search can be described.
	env, err := parseEnv(&raw.Env)
	if err != nil {
		return nil, err
	}
	cfg.Env = env
	return cfg, nil
}

func parseEnv(node *yaml.Node) ([]EnvVar, error) {
	switch {
	case node.Kind == 0, node.Tag == "!!null":
		return nil, nil
	case node.Kind != yaml.MappingNode:
		return nil, fmt.Errorf(`key "env" must be a mapping of names to values, got %s`, describeKind(node.Kind))
	}
	vars := make([]EnvVar, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		name := strings.TrimSpace(key.Value)
		if key.Kind != yaml.ScalarNode || name == "" {
			return nil, fmt.Errorf(`env: variable names must be non-empty strings (line %d)`, key.Line)
		}
		if strings.ContainsRune(name, '=') {
			return nil, fmt.Errorf("env: variable name %q must not contain %q", name, "=")
		}
		if val.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("env %s: value must be a scalar, got %s (line %d)", name, describeKind(val.Kind), val.Line)
		}
		value := val.Value
		if val.Tag == "!!null" {
			value = ""
		}
		vars = append(vars, EnvVar{Name: name, Value: value})
	}
	return vars, nil
}

func describeKind(k yaml.Kind) string {
	switch k {
	case yaml.ScalarNode:
		return "a scalar"
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a mapping"
	case yaml.AliasNode:
		return "an alias"
	default:
		return "nothing"
	}
}

// BaseName is the wrapper's own name with a .exe suffix removed, which is both
// the config file's stem and the name used in error messages.
func BaseName(exePath string) string {
	base := filepath.Base(exePath)
	if ext := filepath.Ext(base); strings.EqualFold(ext, ".exe") {
		base = base[:len(base)-len(ext)]
	}
	return base
}

// ConfigCandidates lists the config paths tried for exePath, in order. The
// lookup is by the executable's own name, so one build serves every tool:
// yt.exe reads yt.yaml, foo.exe reads foo.yaml.
func ConfigCandidates(exePath string) []string {
	dir := filepath.Dir(exePath)
	stem := BaseName(exePath)
	return []string{
		filepath.Join(dir, stem+".yaml"),
		filepath.Join(dir, stem+".yml"),
	}
}

// FindConfig returns the first existing config file for exePath.
func FindConfig(exePath string) (string, error) {
	candidates := ConfigCandidates(exePath)
	for _, path := range candidates {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: looked for %s", ErrNoConfig, strings.Join(candidates, " and "))
}

// LoadConfig finds and parses the config belonging to exePath.
func LoadConfig(exePath string) (*Config, string, error) {
	path, err := FindConfig(exePath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, path, nil
}

// ResolvePath makes a config-supplied path absolute against the wrapper's own
// directory, never the caller's working directory, so a wrapper keeps working
// wherever it is invoked from.
func ResolvePath(path, baseDir string) string {
	p := filepath.FromSlash(path)
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	return filepath.Clean(p)
}

// ResolveTarget turns cfg.Target into an absolute path to an existing file.
func ResolveTarget(target, exePath string) (string, error) {
	resolved := ResolvePath(target, filepath.Dir(exePath))
	candidates := []string{resolved}
	if runtime.GOOS == "windows" && filepath.Ext(resolved) == "" {
		candidates = append(candidates, resolved+".exe")
	}
	for _, candidate := range candidates {
		st, err := os.Stat(candidate)
		if err != nil || st.IsDir() {
			continue
		}
		if sameFile(candidate, exePath) {
			return "", fmt.Errorf("target %s is the wrapper itself, which would loop forever; rename the original and point target at it", candidate)
		}
		return candidate, nil
	}
	return "", fmt.Errorf("target not found: %s", resolved)
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}
