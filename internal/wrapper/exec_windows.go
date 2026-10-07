//go:build windows

package wrapper

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// execTarget spawns the target as a child and waits for it. Windows has no
// exec, so the wrapper stays in the process tree for the duration.
func execTarget(target string, args, env []string) (int, error) {
	cmd := exec.Command(target, args...)
	cmd.Env = env
	// The caller's own quoting is forwarded verbatim where the raw command line
	// is available, instead of being re-escaped from Go's parse of it.
	if tail := rawArgTail(); tail != "" {
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(target) + " " + tail}
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = creationFlags
	cmd.Stdin = usableHandle(os.Stdin)
	cmd.Stdout = usableHandle(os.Stdout)
	cmd.Stderr = usableHandle(os.Stderr)

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return ExitError, err
	}
	return 0, nil
}

// rawArgTail is this process's command line with the program name removed. It
// returns "" if the command line is unavailable or held no arguments, in which
// case os/exec re-escapes the parsed arguments instead.
func rawArgTail() string {
	ptr := syscall.GetCommandLine()
	if ptr == nil {
		return ""
	}
	return StripProgram(utf16PtrToString(ptr))
}

func utf16PtrToString(p *uint16) string {
	n := 0
	for unit := p; *unit != 0; n++ {
		unit = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(unit)) + unsafe.Sizeof(*unit)))
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

// usableHandle drops a standard stream whose handle the OS never gave us. A
// GUI-subsystem process started without redirection has no std handles, and
// passing an invalid one to CreateProcess fails the spawn outright; nil makes
// os/exec substitute the null device.
func usableHandle(f *os.File) *os.File {
	if f == nil {
		return nil
	}
	switch h := syscall.Handle(f.Fd()); h {
	case 0, syscall.InvalidHandle:
		return nil
	}
	return f
}
