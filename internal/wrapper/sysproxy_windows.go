//go:build windows

package wrapper

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// internetSettingsKey is where Windows keeps the per-user proxy configuration,
// the same values the Settings app and Internet Options write.
const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// loadSystemProxy reads the WinInet proxy configuration and translates it.
//
// A missing key, or a missing value within it, is not an error: it means
// nothing is configured, which expands to empty exactly as an unset variable
// does. Only a key that exists but cannot be read is reported.
func loadSystemProxy([]string) (SystemProxy, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return SystemProxy{}, nil
		}
		return SystemProxy{}, fmt.Errorf("reading system proxy settings: %w", err)
	}
	defer key.Close()

	var s WinINetSettings
	if v, _, err := key.GetIntegerValue("ProxyEnable"); err == nil {
		s.ProxyEnable = v != 0
	}
	if v, _, err := key.GetStringValue("ProxyServer"); err == nil {
		s.ProxyServer = v
	}
	if v, _, err := key.GetStringValue("ProxyOverride"); err == nil {
		s.ProxyOverride = v
	}
	return translateWinINet(s), nil
}
