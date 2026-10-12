package wrapper

import (
	"fmt"
	"io"
	"strings"
)

// wrapperNamePrefix is the name the wrapper is published under. While the
// binary still carries it, the command line is the wrapper's own; under any
// other name every argument belongs to the target.
const wrapperNamePrefix = "exe-env-wrapper"

// hasWrapperName reports whether base is still one of the wrapper's own
// names, which is what decides between command mode and wrapper mode.
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

// runCommandMode handles a command line under the wrapper's own name.
//
// Every argument here is the wrapper's, so an unrecognised one is a mistake
// rather than something to forward. Nothing in this path runs a target.
func runCommandMode(args []string, stdout io.Writer) int {
	// Invoked bare there is no tool to run and no config that was ever meant
	// to exist, so help is more use than an error about a missing yaml file.
	if len(args) == 0 {
		renderOverview(stdout)
		return 0
	}

	cmd, ok := lookupCommand(args[0])
	if !ok {
		fmt.Fprintf(stdout, "unrecognised command %q\n\n", args[0])
		renderOverview(stdout)
		return 2
	}

	rest := args[1:]
	opts, err := cmd.parseOptions(rest)
	if err != nil {
		fmt.Fprintf(stdout, "%v\n", err)
		return 2
	}
	operand := firstOperand(rest)

	switch cmd.name {
	case "help":
		return runHelp(stdout, operand)
	case "version":
		printVersion(stdout)
		return 0
	case "setup":
		return report(stdout, runSetup(stdout, opts["--machine"]))
	case "wrap":
		return report(stdout, runWrap(stdout, operand, opts))
	case "adapt":
		return report(stdout, runAdapt(stdout, operand, opts))
	case "check":
		return runCheck(stdout)
	}

	// Unreachable while every command in the table is handled above, and a
	// test walks the table to keep it that way.
	fmt.Fprintf(stdout, "%s is not implemented\n", cmd.name)
	return 2
}

// runHelp prints the overview, or one topic.
func runHelp(stdout io.Writer, topic string) int {
	if topic == "" {
		renderOverview(stdout)
		return 0
	}
	if renderTopic(stdout, topic) {
		return 0
	}
	fmt.Fprintf(stdout, "no help for %q; try one of: %s\n", topic, topicNames())
	return 2
}

// printVersion writes the build version.
func printVersion(w io.Writer) {
	fmt.Fprintf(w, "exe-env-wrapper %s\n", Version)
}

// report turns a command's error into an exit status, printing it first.
func report(w io.Writer, err error) int {
	if err != nil {
		fmt.Fprintf(w, "%v\n", err)
		return 1
	}
	return 0
}

// firstOperand returns the first argument that is not an option.
func firstOperand(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}
