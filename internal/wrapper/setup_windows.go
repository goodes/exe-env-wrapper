//go:build windows

package wrapper

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The two persistent environment stores. The user one needs no elevation; the
// machine one does, and is the only way past a machine PATH entry.
const (
	userEnvKey    = `Environment`
	machineEnvKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
)

// persistentPathStore returns the registry-backed PATH list for the chosen
// scope.
func persistentPathStore(machine bool) (pathStore, error) {
	root, path, scope := registry.CURRENT_USER, userEnvKey, "user"
	if machine {
		root, path, scope = registry.LOCAL_MACHINE, machineEnvKey, "machine"
	}
	return pathStore{
		scope: scope,
		get:   func() ([]string, error) { return readRegistryPath(root, path) },
		set:   func(entries []string) error { return writeRegistryPath(root, path, entries, scope) },
	}, nil
}

// readRegistryPath reads PATH from a registry environment key. A key or value
// that is not there yet is an empty list, not an error: a fresh user profile
// legitimately has no user PATH.
func readRegistryPath(root registry.Key, path string) ([]string, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, nil
		}
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer key.Close()

	// GetStringValue returns the stored text without expanding it, which
	// matters: these values hold things like %USERPROFILE% that must stay
	// unexpanded or every other entry would be rewritten.
	value, _, err := key.GetStringValue("PATH")
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, nil
		}
		return nil, fmt.Errorf("reading PATH from %s: %w", path, err)
	}
	return pathEntries(value), nil
}

// writeRegistryPath writes PATH back, preserving whether it was stored as an
// expandable value, then tells the system so new processes see it.
func writeRegistryPath(root registry.Key, path string, entries []string, scope string) error {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		if scope == "machine" {
			return fmt.Errorf("opening %s for writing (--setup --machine needs administrator rights): %w", path, err)
		}
		return fmt.Errorf("opening %s for writing: %w", path, err)
	}
	defer key.Close()

	// Keep the value's type. Rewriting a REG_EXPAND_SZ as REG_SZ would freeze
	// every %VAR% already in the list as literal text.
	_, valueType, err := key.GetStringValue("PATH")
	expandable := err == nil && valueType == registry.EXPAND_SZ

	joined := joinPathList(entries)
	if expandable {
		err = key.SetExpandStringValue("PATH", joined)
	} else {
		err = key.SetStringValue("PATH", joined)
	}
	if err != nil {
		return fmt.Errorf("writing PATH to %s: %w", path, err)
	}

	broadcastEnvironmentChange()
	return nil
}

// broadcastEnvironmentChange tells running programs the environment moved on.
// Without it, a new terminal started from an already-running Explorer keeps
// the old PATH until sign-out, which looks exactly like --setup having failed.
//
// Best effort: the PATH is already written, so a failure here costs a new
// terminal rather than the change itself.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")
	var result uintptr
	sendMessageTimeout.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(env)),
		uintptr(smtoAbortIfHung),
		uintptr(5000),
		uintptr(unsafe.Pointer(&result)),
	)
}
