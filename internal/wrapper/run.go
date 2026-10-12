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

// Run picks which of the two programs this is, by the name it was invoked
// under.
//
// Under its own name the command line is the wrapper's own and nothing is run.
// Under any other name the wrapper stands in for a tool, and every argument
// belongs to that tool, so its --help, --version and its own subcommands all
// still reach it.
//
// Only the console build takes commands. The windowless build has no console
// to print to and is a runtime payload, installed by the console build.
func Run() int {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "exe-env-wrapper: cannot determine own path: %v\n", err)
		return ExitError
	}

	if ownFlagsEnabled && hasWrapperName(BaseName(exePath)) {
		return runCommandMode(os.Args[1:], os.Stdout)
	}
	return runWrapperMode(exePath)
}

// runWrapperMode is the wrapper proper: read the config named after this
// executable, apply its environment, and hand off to the target. The returned
// value is the target's exit code, or ExitError if it could not get that far.
func runWrapperMode(exePath string) int {
	rep := &reporter{stderr: os.Stderr, name: BaseName(exePath)}
	if path := os.Getenv(LogEnvVar); path != "" {
		rep.logPath = ResolvePath(path, filepath.Dir(exePath))
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
