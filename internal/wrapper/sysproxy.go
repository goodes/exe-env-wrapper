package wrapper

import "strings"

// WinINetSettings holds the raw per-user proxy configuration as Windows stores
// it under Internet Settings. Keeping it as plain data lets the translation
// below be tested on any platform, away from the registry.
type WinINetSettings struct {
	// ProxyEnable reports whether the system is set to use ProxyServer at all.
	ProxyEnable bool
	// ProxyServer is either a bare host:port or a per-scheme list such as
	// "http=a:1;https=b:2".
	ProxyServer string
	// ProxyOverride is the bypass list, semicolon-separated, possibly including
	// the <local> token.
	ProxyOverride string
}

// SystemProxy is the translated form: the three values as proxy-aware tools
// expect to find them in the environment.
type SystemProxy struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

// translateWinINet converts Windows' proxy settings into the environment
// variable forms. Windows and the proxy environment variables disagree on
// nearly every detail: separators, scheme prefixes, and the <local> token.
func translateWinINet(s WinINetSettings) SystemProxy {
	var out SystemProxy
	if s.ProxyEnable {
		out.HTTPProxy, out.HTTPSProxy = parseProxyServer(s.ProxyServer)
	}
	// Deliberately outside the ProxyEnable check: see the test.
	out.NoProxy = parseProxyOverride(s.ProxyOverride)
	return out
}

// localTokenHosts is what <local> becomes. The token means "bypass for
// addresses without a dot", which NO_PROXY cannot express, so it expands to
// the loopback names every client already understands.
var localTokenHosts = []string{"localhost", "127.0.0.1", "::1"}

// parseProxyOverride converts the bypass list to NO_PROXY form: commas rather
// than semicolons, <local> expanded, blank entries dropped, order kept.
func parseProxyOverride(v string) string {
	var entries []string
	seen := make(map[string]bool)
	add := func(entry string) {
		if entry == "" || seen[entry] {
			return
		}
		seen[entry] = true
		entries = append(entries, entry)
	}
	for _, part := range strings.Split(v, ";") {
		part = strings.TrimSpace(part)
		if strings.EqualFold(part, "<local>") {
			for _, host := range localTokenHosts {
				add(host)
			}
			continue
		}
		add(part)
	}
	return strings.Join(entries, ",")
}

// parseProxyServer handles both forms Windows stores: a bare host:port that
// applies to every scheme, and a per-scheme list such as
// "http=a:1;https=b:2;socks=c:3". Schemes with no environment variable to go
// in, socks among them, are skipped.
func parseProxyServer(v string) (httpProxy, httpsProxy string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", ""
	}
	if !strings.Contains(v, "=") {
		proxy := withScheme(v)
		return proxy, proxy
	}
	for _, part := range strings.Split(v, ";") {
		scheme, addr, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(scheme)) {
		case "http":
			httpProxy = withScheme(addr)
		case "https":
			httpsProxy = withScheme(addr)
		}
	}
	return httpProxy, httpsProxy
}

// withScheme prefixes http:// unless the value already carries a scheme.
// Windows stores a bare host:port; proxy environment variables are read as
// URLs.
func withScheme(v string) string {
	if strings.Contains(v, "://") {
		return v
	}
	return "http://" + v
}

// envAnyCase finds name in an os.Environ()-style slice, matching either case.
// Linux conventionally uses lowercase no_proxy while Windows-born configs use
// NO_PROXY, and both are found in the wild.
func envAnyCase(env []string, name string) string {
	for _, entry := range env {
		if n, v, ok := splitEntry(entry); ok && strings.EqualFold(n, name) {
			return v
		}
	}
	return ""
}

// collapseListSeparators drops the empty entries from a comma-separated value.
//
// It applies only to values that used a system name. Those routinely expand to
// nothing: a machine with no proxy configured turns the documented
// NO_PROXY: ${system.no_proxy},localhost into ",localhost". An ordinary ${VAR}
// in the same position keeps its stray separator, as it does in a shell.
func collapseListSeparators(v string) string {
	if !strings.Contains(v, ",") {
		return v
	}
	parts := strings.Split(v, ",")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, ",")
}
