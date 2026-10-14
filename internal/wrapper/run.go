package wrapper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ExitError is the status the wrapper returns when it cannot start the target
// at all, as opposed to a status the target itself produced.
const ExitError = 125

// Version now lives in version.go, derived from the stamp that makes it
// readable out of a compiled binary.

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
	env, changes, err := BuildEnvWithChanges(os.Environ(), cfg.Env)
	if err != nil {
		rep.report(fmt.Errorf("%s: %w", cfgPath, err))
		return ExitError
	}

	traceFile, traceLevel, warn := resolveTrace(cfg.Trace, cfg.TraceLevel,
		os.Getenv(TraceEnvVar), os.Getenv(TraceLevelEnvVar))
	if warn != "" {
		fmt.Fprintf(rep.stderr, "%s: %s\n", rep.name, warn)
	}

	code, err := runTarget(traceFile, traceLevel, exePath, target, env, changes, rep)
	if err != nil {
		rep.report(fmt.Errorf("cannot run %s: %w", target, err))
		return ExitError
	}
	return code
}

// runTarget hands off to the target, writing as much to the trace as the
// level asks for.
//
// Only level 2 and above puts the wrapper in the data path. Below that the
// child inherits the standard handles directly and, on Unix, the wrapper is
// replaced by the target altogether, so nothing sits between the caller and
// the tool.
func runTarget(traceFile string, level int, exePath, target string, env []string, changes []EnvChange, rep *reporter) (int, error) {
	args := os.Args[1:]
	if level == TraceOff || traceFile == "" {
		return execTarget(target, args, env)
	}

	path := ResolvePath(traceFile, filepath.Dir(exePath))
	sink, err := newTraceSink(path, rep.name, rep.stderr)
	if err != nil {
		// A trace that cannot be opened is reported, and the tool still runs
		// untraced. Losing a debugging aid must not take the tool down.
		rep.report(err)
		return execTarget(target, args, env)
	}
	defer sink.Close()

	// Written before the target starts, so a run that then hung or was killed
	// still appears.
	sink.recordLine("run " + target + " " + renderArgs(args))

	if level >= TraceEnv {
		for _, change := range changes {
			sink.recordLine(describeEnvChange(change))
		}
	}

	if level >= TraceStreams {
		return execTargetTraced(target, args, env, sink)
	}
	return execTarget(target, args, env)
}

// describeEnvChange renders one variable's before and after for a level 3
// trace.
func describeEnvChange(c EnvChange) string {
	before := "(unset)"
	if c.HadBefore {
		before = strconv.Quote(c.Before)
	}
	if !c.Applied {
		// The case a user would otherwise find baffling: a variable they
		// configured that is not set, because the value came to nothing and
		// blanking an inherited one would have been worse.
		return fmt.Sprintf("env %s: left as %s (value expanded to nothing)", c.Name, before)
	}
	return fmt.Sprintf("env %s: %s -> %s", c.Name, before, strconv.Quote(c.After))
}

// reporter writes wrapper failures to stderr and, when a log path is known, to
// that file as well. The windowless build has no usable stderr, so the file is
// often the only trace of a failure.
type reporter struct {
	stderr  io.Writer
	name    string
	logPath string
}

// renderArgs makes an argument list readable in a log line.
func renderArgs(args []string) string {
	if raw := rawCommandTail(); raw != "" {
		return raw
	}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" || strings.ContainsAny(arg, " \t\"") {
			quoted = append(quoted, strconv.Quote(arg))
			continue
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}

func (r *reporter) report(err error) {
	fmt.Fprintf(r.stderr, "%s: %v\n", r.name, err)
	r.write(err.Error())
}

// write appends one line to the log file, if one is configured.
func (r *reporter) write(message string) {
	if r.logPath == "" {
		return
	}
	f, openErr := os.OpenFile(r.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		fmt.Fprintf(r.stderr, "%s: cannot write log %s: %v\n", r.name, r.logPath, openErr)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s (exe-env-wrapper %s): %s\n",
		time.Now().Format(time.RFC3339), r.name, Version, message)
}
