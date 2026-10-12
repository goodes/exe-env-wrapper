package wrapper

import (
	"fmt"
	"runtime"
	"strings"
)

// BuildEnv applies vars on top of base, an os.Environ()-style slice. Variables
// that vars does not mention are inherited unchanged, and each value may refer
// through ${VAR} to whatever is visible at that point: the inherited
// environment plus any earlier entry in the same block.
func BuildEnv(base []string, vars []EnvVar) ([]string, error) {
	return buildEnv(base, vars, loadSystemProxy)
}

// systemPrefix marks the names answered by the operating system rather than by
// the inherited environment. An environment variable name cannot contain a
// dot, so the namespace cannot collide with a real one.
const systemPrefix = "system."

// systemProxyFields maps each valid system.* name to the value it answers
// from. Looking the name up before the settings are read means a typo is
// reported without touching the registry, and reported as a typo rather than
// as an empty value.
var systemProxyFields = map[string]func(SystemProxy) string{
	systemPrefix + "http_proxy":  func(s SystemProxy) string { return s.HTTPProxy },
	systemPrefix + "https_proxy": func(s SystemProxy) string { return s.HTTPSProxy },
	systemPrefix + "no_proxy":    func(s SystemProxy) string { return s.NoProxy },
}

// buildEnv is BuildEnv with the system proxy source injected, so the
// expansion can be tested without a registry.
func buildEnv(base []string, vars []EnvVar, load func([]string) (SystemProxy, error)) ([]string, error) {
	out := make([]string, len(base))
	copy(out, base)

	index := make(map[string]int, len(out))
	for i, entry := range out {
		if name, _, ok := splitEntry(entry); ok {
			index[envKey(name)] = i
		}
	}
	// The system settings are read at most once, and only if a system.* name
	// actually appears: most configs never mention one.
	var (
		sys        SystemProxy
		sysLoaded  bool
		usedSystem bool
	)
	lookup := func(name string) (string, error) {
		if strings.HasPrefix(name, systemPrefix) {
			field, ok := systemProxyFields[name]
			if !ok {
				return "", fmt.Errorf(
					"unknown ${%s}: valid names are ${system.http_proxy}, ${system.https_proxy} and ${system.no_proxy}",
					name)
			}
			if !sysLoaded {
				loaded, err := load(base)
				if err != nil {
					return "", err
				}
				sys, sysLoaded = loaded, true
			}
			usedSystem = true
			return field(sys), nil
		}
		if i, ok := index[envKey(name)]; ok {
			_, value, _ := splitEntry(out[i])
			return value, nil
		}
		return "", nil
	}

	for _, v := range vars {
		usedSystem = false
		value, err := Expand(v.Value, lookup)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", v.Name, err)
		}
		if usedSystem {
			value = collapseListSeparators(value)
		}
		entry := v.Name + "=" + value
		if i, ok := index[envKey(v.Name)]; ok {
			out[i] = entry
			continue
		}
		index[envKey(v.Name)] = len(out)
		out = append(out, entry)
	}
	return out, nil
}

// envKey normalises a variable name for lookup. Windows environment names are
// case-insensitive, so an HTTP_PROXY override has to replace an inherited
// http_proxy rather than sit beside it.
func envKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

func splitEntry(entry string) (name, value string, ok bool) {
	i := strings.IndexByte(entry, '=')
	if i <= 0 {
		// Windows exposes hidden per-drive entries such as "=C:=C:\dir". They are
		// passed through untouched rather than treated as a variable named "".
		return "", "", false
	}
	return entry[:i], entry[i+1:], true
}

// Expand replaces every ${VAR} in s with lookup's answer. A variable that is
// not set expands to the empty string, as in a shell. A doubled $$ is a literal
// $, so $${HOME} survives as ${HOME}, and a bare $ not followed by { is literal
// too: Windows paths and proxy passwords use it freely.
func Expand(s string, lookup func(string) (string, error)) (string, error) {
	if !strings.ContainsRune(s, '$') {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		rest := s[i+1:]
		switch {
		case strings.HasPrefix(rest, "$"):
			b.WriteByte('$')
			i += 2
		case strings.HasPrefix(rest, "{"):
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated ${ in %q", s)
			}
			name := rest[1:end]
			if strings.TrimSpace(name) == "" {
				return "", fmt.Errorf("empty ${} in %q", s)
			}
			value, err := lookup(name)
			if err != nil {
				return "", err
			}
			b.WriteString(value)
			i += 1 + end + 1
		default:
			b.WriteByte('$')
			i++
		}
	}
	return b.String(), nil
}
