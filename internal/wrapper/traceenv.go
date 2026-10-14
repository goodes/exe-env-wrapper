package wrapper

import (
	"fmt"
	"strconv"
	"strings"
)

// TraceEnvVar and TraceLevelEnvVar name a trace from the environment.
//
// They exist because the installation wrap produces has no config file at all:
// that is the point of it, and writing one by hand to add a trace would also
// replace the built-in defaults, changing the thing being investigated. These
// let any wrapper be observed without touching it, as EXE_ENV_WRAPPER_LOG
// already does for the log.
const (
	TraceEnvVar      = "EXE_ENV_WRAPPER_TRACE"
	TraceLevelEnvVar = "EXE_ENV_WRAPPER_TRACE_LEVEL"
)

// resolveTrace settles the trace file and level from the config and the
// environment, returning a warning for anything it had to ignore.
//
// The environment wins over the config. It is reached for in order to watch a
// wrapper that is already misbehaving, so it would be no use if the config
// could override it.
//
// Nothing here fails the run: a trace is a debugging aid, and a mistake in one
// must not stop the tool it was meant to help with. A config file's own
// trace_level is validated at parse time, where it can be a proper error.
func resolveTrace(cfgFile string, cfgLevel int, envFile, envLevel string) (file string, level int, warn string) {
	file, level = cfgFile, cfgLevel

	if envFile = strings.TrimSpace(envFile); envFile != "" {
		file = envFile
		// A file named in the environment with no level means level 1, the
		// same default a config file gets.
		if level == TraceOff {
			level = TraceCommand
		}
	}

	envLevel = strings.TrimSpace(envLevel)
	if envLevel == "" {
		if file == "" {
			return "", TraceOff, ""
		}
		return file, level, ""
	}

	parsed, err := strconv.Atoi(envLevel)
	switch {
	case err != nil:
		return file, level, fmt.Sprintf("%s is %q, which is not a number; ignoring it", TraceLevelEnvVar, envLevel)
	case parsed < TraceOff || parsed > TraceEnv:
		return file, level, fmt.Sprintf("%s is %d, want %d to %d; ignoring it",
			TraceLevelEnvVar, parsed, TraceOff, TraceEnv)
	case file == "":
		return "", TraceOff, fmt.Sprintf("%s is set but there is nowhere to write it; set %s or \"trace\" in the config",
			TraceLevelEnvVar, TraceEnvVar)
	}
	return file, parsed, ""
}
