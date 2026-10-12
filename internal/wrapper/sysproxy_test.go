package wrapper

import (
	"strings"
	"testing"
)

// A bare host:port in ProxyServer applies to every scheme, and gains the
// http:// prefix that proxy environment variables are consumed as.
func TestTranslateWinINetBareProxyServer(t *testing.T) {
	got := translateWinINet(WinINetSettings{
		ProxyEnable: true,
		ProxyServer: "proxy.example:8080",
	})

	if want := "http://proxy.example:8080"; got.HTTPProxy != want {
		t.Errorf("HTTPProxy = %q, want %q", got.HTTPProxy, want)
	}
	if want := "http://proxy.example:8080"; got.HTTPSProxy != want {
		t.Errorf("HTTPSProxy = %q, want %q", got.HTTPSProxy, want)
	}
}

// ProxyServer may instead carry a per-scheme list. Each scheme takes its own
// proxy, and schemes the environment variables have no slot for are ignored.
func TestTranslateWinINetPerSchemeProxyServer(t *testing.T) {
	got := translateWinINet(WinINetSettings{
		ProxyEnable: true,
		ProxyServer: "http=a.example:1;https=b.example:2;socks=c.example:3",
	})

	if want := "http://a.example:1"; got.HTTPProxy != want {
		t.Errorf("HTTPProxy = %q, want %q", got.HTTPProxy, want)
	}
	if want := "http://b.example:2"; got.HTTPSProxy != want {
		t.Errorf("HTTPSProxy = %q, want %q", got.HTTPSProxy, want)
	}
}

// ProxyOverride is semicolon-separated and may carry the <local> token, which
// NO_PROXY has no equivalent for. Order is preserved and empty entries, which
// a trailing or doubled semicolon leaves behind, are dropped.
func TestTranslateWinINetOverrideBecomesNoProxy(t *testing.T) {
	got := translateWinINet(WinINetSettings{
		ProxyEnable:   true,
		ProxyServer:   "proxy.example:8080",
		ProxyOverride: "internal.example;*.corp.example;<local>;;10.0.0.1",
	})

	want := "internal.example,*.corp.example,localhost,127.0.0.1,::1,10.0.0.1"
	if got.NoProxy != want {
		t.Errorf("NoProxy = %q, want %q", got.NoProxy, want)
	}
}

// ProxyEnable=0 means the system is not using its proxy, so the two proxy
// values go empty. The bypass list is still returned: taking only the bypass
// list from the system while naming the proxy by hand is the case this whole
// feature exists for.
func TestTranslateWinINetDisabledKeepsBypassList(t *testing.T) {
	got := translateWinINet(WinINetSettings{
		ProxyEnable:   false,
		ProxyServer:   "proxy.example:8080",
		ProxyOverride: "internal.example",
	})

	if got.HTTPProxy != "" {
		t.Errorf("HTTPProxy = %q, want empty", got.HTTPProxy)
	}
	if got.HTTPSProxy != "" {
		t.Errorf("HTTPSProxy = %q, want empty", got.HTTPSProxy)
	}
	if want := "internal.example"; got.NoProxy != want {
		t.Errorf("NoProxy = %q, want %q", got.NoProxy, want)
	}
}

// Edge cases the spec calls out. These cover branches written during earlier
// GREEN steps rather than driven by a failing test of their own, so they are
// regression cover, not specification.
func TestTranslateWinINetEdgeCases(t *testing.T) {
	tests := []struct {
		name                             string
		in                               WinINetSettings
		wantHTTP, wantHTTPS, wantNoProxy string
	}{
		{
			name:     "a stored scheme is not doubled",
			in:       WinINetSettings{ProxyEnable: true, ProxyServer: "http=http://a:1;https=https://b:2"},
			wantHTTP: "http://a:1", wantHTTPS: "https://b:2",
		},
		{
			name:        "local token does not duplicate an explicit loopback entry",
			in:          WinINetSettings{ProxyOverride: "localhost;<local>"},
			wantNoProxy: "localhost,127.0.0.1,::1",
		},
		{
			name: "nothing configured yields nothing",
			in:   WinINetSettings{},
		},
		{
			name: "enabled with an empty server yields no proxy",
			in:   WinINetSettings{ProxyEnable: true, ProxyServer: ""},
		},
		{
			name:     "a per-scheme list naming only http leaves https empty",
			in:       WinINetSettings{ProxyEnable: true, ProxyServer: "http=a:1"},
			wantHTTP: "http://a:1",
		},
		{
			name:        "whitespace around entries is trimmed",
			in:          WinINetSettings{ProxyOverride: " a.example ; b.example "},
			wantNoProxy: "a.example,b.example",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateWinINet(tt.in)
			if got.HTTPProxy != tt.wantHTTP {
				t.Errorf("HTTPProxy = %q, want %q", got.HTTPProxy, tt.wantHTTP)
			}
			if got.HTTPSProxy != tt.wantHTTPS {
				t.Errorf("HTTPSProxy = %q, want %q", got.HTTPSProxy, tt.wantHTTPS)
			}
			if got.NoProxy != tt.wantNoProxy {
				t.Errorf("NoProxy = %q, want %q", got.NoProxy, tt.wantNoProxy)
			}
		})
	}
}

// fixedSystemProxy is a loader that answers with settings the test chose,
// standing in for the registry.
func fixedSystemProxy(sys SystemProxy) func([]string) (SystemProxy, error) {
	return func([]string) (SystemProxy, error) { return sys, nil }
}

// The three system names expand like any other value, and compose with
// literal text around them.
func TestBuildEnvExpandsSystemProxyNames(t *testing.T) {
	load := fixedSystemProxy(SystemProxy{
		HTTPProxy:  "http://proxy.example:8080",
		HTTPSProxy: "http://proxy.example:8443",
		NoProxy:    "internal.example",
	})

	out, err := buildEnv(nil, []EnvVar{
		{Name: "HTTP_PROXY", Value: "${system.http_proxy}"},
		{Name: "HTTPS_PROXY", Value: "${system.https_proxy}"},
		{Name: "NO_PROXY", Value: "${system.no_proxy},localhost"},
	}, load)
	if err != nil {
		t.Fatalf("buildEnv: %v", err)
	}

	for name, want := range map[string]string{
		"HTTP_PROXY":  "http://proxy.example:8080",
		"HTTPS_PROXY": "http://proxy.example:8443",
		"NO_PROXY":    "internal.example,localhost",
	} {
		got, ok := valueOf(t, out, name)
		if !ok {
			t.Errorf("%s not set", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// A misspelled system.* name is a config mistake, not an absent variable. It
// fails the run rather than expanding to empty, which would otherwise show up
// as a proxy setting that silently does nothing.
func TestBuildEnvRejectsUnknownSystemName(t *testing.T) {
	_, err := buildEnv(nil, []EnvVar{
		{Name: "NO_PROXY", Value: "${system.no_proxyy}"},
	}, fixedSystemProxy(SystemProxy{}))

	if err == nil {
		t.Fatal("buildEnv accepted an unknown system.* name, want an error")
	}
	if !strings.Contains(err.Error(), "system.no_proxyy") {
		t.Errorf("error %q should name the offending key", err)
	}
	if !strings.Contains(err.Error(), "system.no_proxy") {
		t.Errorf("error %q should list the valid names", err)
	}
}

// The settings are read only when a system.* name actually appears, and then
// once however many names reference them. Most configs mention none, and a
// wrapper runs on every invocation of the tool it wraps.
func TestBuildEnvReadsSystemSettingsLazilyAndOnce(t *testing.T) {
	calls := 0
	load := func([]string) (SystemProxy, error) {
		calls++
		return SystemProxy{NoProxy: "internal.example"}, nil
	}

	if _, err := buildEnv(nil, []EnvVar{{Name: "PATH", Value: "/usr/bin"}}, load); err != nil {
		t.Fatalf("buildEnv: %v", err)
	}
	if calls != 0 {
		t.Errorf("read the settings %d times for a config that never mentions them, want 0", calls)
	}

	calls = 0
	if _, err := buildEnv(nil, []EnvVar{
		{Name: "A", Value: "${system.no_proxy}"},
		{Name: "B", Value: "${system.http_proxy}"},
		{Name: "C", Value: "${system.no_proxy}"},
	}, load); err != nil {
		t.Fatalf("buildEnv: %v", err)
	}
	if calls != 1 {
		t.Errorf("read the settings %d times for three references, want 1", calls)
	}
}

// A system name that expands to nothing must not leave a stray separator
// behind: NO_PROXY: ${system.no_proxy},localhost is the documented pattern,
// and on a machine with no proxy configured it would otherwise produce
// ",localhost".
func TestBuildEnvCollapsesEmptySystemListEntries(t *testing.T) {
	load := fixedSystemProxy(SystemProxy{})

	out, err := buildEnv(nil, []EnvVar{
		{Name: "NO_PROXY", Value: "${system.no_proxy},localhost"},
		{Name: "BOTH_ENDS", Value: "${system.no_proxy},a,${system.no_proxy},b,${system.no_proxy}"},
	}, load)
	if err != nil {
		t.Fatalf("buildEnv: %v", err)
	}

	for name, want := range map[string]string{
		"NO_PROXY":  "localhost",
		"BOTH_ENDS": "a,b",
	} {
		got, ok := valueOf(t, out, name)
		if !ok {
			t.Errorf("%s not set", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// The collapse applies only where a system name was used. An unset ${VAR} in
// the same position keeps its stray separator, as it does in a shell and as it
// always has here.
func TestBuildEnvLeavesOrdinaryEmptyExpansionsAlone(t *testing.T) {
	out, err := buildEnv(nil, []EnvVar{
		{Name: "NO_PROXY", Value: "${NOT_SET},localhost"},
	}, fixedSystemProxy(SystemProxy{}))
	if err != nil {
		t.Fatalf("buildEnv: %v", err)
	}

	got, ok := valueOf(t, out, "NO_PROXY")
	if !ok {
		t.Fatal("NO_PROXY not set")
	}
	if want := ",localhost"; got != want {
		t.Errorf("NO_PROXY = %q, want %q", got, want)
	}
}
