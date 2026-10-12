package wrapper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ExitError is the status the wrapper returns when it cannot start the target
// at all, as opposed to a status the target itself produced.
const ExitError = 125

// Version is set at build time with -ldflags "-X ...wrapper.Version=v1.2.3".
var Version = "dev"

// LogEnvVar names a log file for the bootstrap phase, before the config has
// been read. A broken config in the windowless build would otherwise fail with
// nowhere to say so.
const LogEnvVar = "EXE_ENV_WRAPPER_LOG"

// Run is the whole program: read the config named after this executable, apply
// its environment, and hand off to the target. The returned value is the
// target's exit code, or ExitError if the wrapper could not get that far.
func Run() int {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "exe-env-wrapper: cannot determine own path: %v\n", err)
		return ExitError
	}

	rep := &reporter{stderr: os.Stderr, name: BaseName(exePath)}
	if path := os.Getenv(LogEnvVar); path != "" {
		rep.logPath = ResolvePath(path, filepath.Dir(exePath))
	}

	// Under its own name, and only in the build that has a console to print
	// to, an argument may be the wrapper's own. Under any other name every
	// argument belongs to the target, so a wrapped tool's --help stays its own.
	if ownFlagsEnabled && hasWrapperName(rep.name) {
		if code, handled := handleOwnFlags(os.Args[1:], os.Stdout); handled {
			return code
		}
	}

	cfg, cfgPath, err := LoadConfig(exePath)
	switch {
	case errors.Is(err, ErrNoConfig):
		// No config file is the normal state for a wrapper in the wrapper
		// folder: it runs on the defaults and takes its target from PATH.
		// Having nothing to hand off to is reported below, by the resolution
		// that can describe what it searched.
		cfg, cfgPath = DefaultConfig(), "built-in defaults"
	case err != nil:
		rep.report(err)
		return ExitError
	}
	if cfg.Log != "" {
		rep.logPath = ResolvePath(cfg.Log, filepath.Dir(exePath))
	}

	target, err := resolveTargetFor(cfg, exePath)
	if err != nil {
		rep.report(fmt.Errorf("%s: %w", cfgPath, err))
		return ExitError
	}
	env, err := BuildEnv(os.Environ(), cfg.Env)
	if err != nil {
		rep.report(fmt.Errorf("%s: %w", cfgPath, err))
		return ExitError
	}

	code, err := execTarget(target, os.Args[1:], env)
	if err != nil {
		rep.report(fmt.Errorf("cannot run %s: %w", target, err))
		return ExitError
	}
	return code
}

// reporter writes wrapper failures to stderr and, when a log path is known, to
// that file as well. The windowless build has no usable stderr, so the file is
// often the only trace of a failure.
type reporter struct {
	stderr  io.Writer
	name    string
	logPath string
}

func (r *reporter) report(err error) {
	fmt.Fprintf(r.stderr, "%s: %v\n", r.name, err)
	if r.logPath == "" {
		return
	}
	f, openErr := os.OpenFile(r.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		fmt.Fprintf(r.stderr, "%s: cannot write log %s: %v\n", r.name, r.logPath, openErr)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s (exe-env-wrapper %s): %v\n",
		time.Now().Format(time.RFC3339), r.name, Version, err)
}
