//go:build !windows

package wrapper

import (
	"errors"
	"fmt"
	"os"
)

// persistentPathStore has nothing to write to here. There is no system-wide
// place a process can change PATH for later logins: it lives in whichever
// shell profile the user keeps, which is theirs to edit and not ours to guess
// at. The store reads the current PATH so --setup can still report ordering,
// and refuses the write with the line to add.
func persistentPathStore(machine bool) (pathStore, error) {
	if machine {
		return pathStore{}, errors.New("--machine is a Windows notion; on this platform PATH lives in your shell profile")
	}
	return pathStore{
		scope: "shell profile",
		get:   func() ([]string, error) { return pathEntries(os.Getenv("PATH")), nil },
		set:   func([]string) error { return errShellProfile() },
	}, nil
}

func errShellProfile() error {
	dir, err := wrapperFolder()
	if err != nil {
		return err
	}
	return fmt.Errorf("add this to your shell profile, then open a new shell:\n\n    export PATH=\"%s:$PATH\"\n", dir)
}
