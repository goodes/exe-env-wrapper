//go:build windows

package wrapper

import "testing"

// Exercises the real registry read. It asserts only that the plumbing works,
// because what is configured is whatever the machine running the test happens
// to have: a machine with no proxy at all is the normal case on a CI runner
// and must not fail.
func TestLoadSystemProxyReadsTheRegistry(t *testing.T) {
	got, err := loadSystemProxy(nil)
	if err != nil {
		t.Fatalf("loadSystemProxy: %v", err)
	}
	t.Logf("this machine: http_proxy=%q https_proxy=%q no_proxy=%q",
		got.HTTPProxy, got.HTTPSProxy, got.NoProxy)
}
