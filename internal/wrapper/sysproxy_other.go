//go:build !windows

package wrapper

// loadSystemProxy answers from the inherited environment on platforms with no
// system proxy store to read.
//
// This keeps one config file portable: ${system.no_proxy} resolves to
// something sensible on Linux rather than failing, and a wrapper config
// written on Windows works unchanged when the same tool is wrapped there.
func loadSystemProxy(base []string) (SystemProxy, error) {
	return SystemProxy{
		HTTPProxy:  envAnyCase(base, "http_proxy"),
		HTTPSProxy: envAnyCase(base, "https_proxy"),
		NoProxy:    envAnyCase(base, "no_proxy"),
	}, nil
}
