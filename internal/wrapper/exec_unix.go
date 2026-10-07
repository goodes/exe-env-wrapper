//go:build !windows

package wrapper

import "syscall"

// execTarget replaces the wrapper process with the target. One less process in
// the tree, and signals, job control and the exit code reach the caller
// directly. It only returns if the exec itself failed.
func execTarget(target string, args, env []string) (int, error) {
	argv := make([]string, 0, len(args)+1)
	argv = append(argv, target)
	argv = append(argv, args...)
	return ExitError, syscall.Exec(target, argv, env)
}
