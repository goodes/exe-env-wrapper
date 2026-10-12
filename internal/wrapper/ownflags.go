package wrapper

import (
	"fmt"
	"io"
	"strings"
)

// wrapperNamePrefix is the name the wrapper is published under. While the
// binary still carries it, arguments are the wrapper's own; under any other
// name every argument belongs to the target.
const wrapperNamePrefix = "exe-env-wrapper"

// hasWrapperName reports whether base is still one of the wrapper's own
// names.
//
// The match is on the prefix rather than the exact name because the published
// downloads are named exe-env-wrapper-w-windows-amd64 and the like, and that
// is what gets invoked straight out of a downloads folder. Matching is
// case-insensitive because Windows file names are.
func hasWrapperName(base string) bool {
	lower := strings.ToLower(base)
	if !strings.HasPrefix(lower, wrapperNamePrefix) {
		return false
	}
	// Only the bare name or a suffixed variant of it: "my-exe-env-wrapper" is
	// somebody else's tool, and the prefix check alone would claim it.
	rest := lower[len(wrapperNamePrefix):]
	return rest == "" || strings.HasPrefix(rest, "-")
}

// usage is what --help prints, and what a bare invocation under the wrapper's
// own name prints instead of complaining about a config file that was never
// meant to exist.
const usage = `exe-env-wrapper sets environment variables from a YAML file named after
itself, then runs a target binary with every argument passed straight through.

Under its own name it takes the flags below. Renamed to stand in for a tool,
it takes none: every argument belongs to that tool.

  --wrap <name|path>   Install a wrapper for a tool into the wrapper folder,
                       leaving the original untouched. A bare name is resolved
                       on PATH and stays dynamic, so the tool can be upgraded
                       or moved without breaking the wrapper. A full path is
                       pinned in the config.
  --adapt <path>       Replace a binary in place: rename it aside with a hash
                       of its contents and stand in for it under its old name.
                       For callers that hard-code the path, where PATH is
                       never consulted. The wrapper folder is the better
                       option wherever it works, because an installer that
                       regenerates the binary will overwrite an adapted one.
  --setup              Create the wrapper folder and put it on PATH.
  --check              Report what each installed wrapper resolves to.
  --help, --version    This text, and the build version.

Config file (<wrapper name>.yaml, beside the wrapper):

  target: yt-cli.exe        Optional. Absolute, or relative to the wrapper.
                            Omitted means "run the next match for my own name
                            on PATH", which is how a wrapper in the wrapper
                            folder shadows a tool without naming its location.
  env:                      Optional. Variables to set or override; anything
    HTTP_PROXY: http://p:1  not named here is inherited unchanged.
  log: wrapper.log          Optional. Where the wrapper records its own
                            failures. Worth setting for the windowless build,
                            whose stderr usually goes nowhere.

Values may refer to ${VAR}, resolved against the inherited environment plus
any earlier entry in the same block. ${system.http_proxy},
${system.https_proxy} and ${system.no_proxy} come from the operating system
instead: the bypass list especially, which clients built on httpx never pick
up on their own.

With no config file at all, a wrapper applies the system proxy settings and
resolves its target from PATH, so installing one is a single copy.

On Windows there are two builds. The console build is for interactive use and
for agents that already own a console; exe-env-wrapper-w never gets a console,
so no window flashes when a background agent spawns it. Only the console build
takes the flags above.
`

// printUsage writes the help text.
func printUsage(w io.Writer) {
	fmt.Fprint(w, usage)
}

// printVersion writes the build version.
func printVersion(w io.Writer) {
	fmt.Fprintf(w, "exe-env-wrapper %s\n", Version)
}

// handleOwnFlags deals with an argument that belongs to the wrapper rather
// than to the target, reporting whether it did.
//
// Reached only under the wrapper's own name and only in the console build, so
// a wrapped tool's own --help and --version are never intercepted.
func handleOwnFlags(args []string, stdout io.Writer) (code int, handled bool) {
	// Invoked bare under its own name, there is no tool to run and no config
	// that was ever meant to exist. Help is more use than an error about a
	// missing exe-env-wrapper.yaml.
	if len(args) == 0 {
		printUsage(stdout)
		return 0, true
	}
	switch args[0] {
	case "--help", "-h":
		printUsage(stdout)
		return 0, true
	case "--version":
		printVersion(stdout)
		return 0, true
	case "--setup":
		return report(stdout, runSetup(stdout, hasFlag(args[1:], "--machine"))), true
	case "--wrap":
		return report(stdout, runWrap(stdout, args[1:])), true
	case "--adapt":
		return report(stdout, runAdapt(stdout, args[1:])), true
	case "--check":
		return runCheck(stdout), true
	}

	// Under its own name there is no tool these could belong to, so an
	// unrecognised argument is a mistake rather than something to pass on.
	fmt.Fprintf(stdout, "unrecognised argument %q\n\n", args[0])
	printUsage(stdout)
	return 2, true
}

// report turns a command's error into an exit status, printing it first.
func report(w io.Writer, err error) int {
	if err != nil {
		fmt.Fprintf(w, "%v\n", err)
		return 1
	}
	return 0
}

// hasFlag reports whether args contains name.
func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

// firstOperand returns the first argument that is not a flag.
func firstOperand(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}
