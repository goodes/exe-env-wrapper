package wrapper

import (
	"strings"
	"testing"
)

// The regression that matters most. Under an adapted name every argument
// belongs to the tool, so --help has to reach it: a wrapper that swallowed
// --help would break the help output of every tool it stands in for.
func TestEndToEndHelpReachesTheTargetUnderAnAdaptedName(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\n")
	res := run(t, wrapper, "", nil, "--help")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ARG:--help") {
		t.Errorf("stdout = %q, want --help to have reached the target", res.stdout)
	}
	if strings.Contains(res.stdout, "exe-env-wrapper sets environment variables") {
		t.Error("the wrapper printed its own help instead of passing --help through")
	}
}

// --version likewise belongs to the tool under an adapted name.
func TestEndToEndVersionReachesTheTargetUnderAnAdaptedName(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\n")
	res := run(t, wrapper, "", nil, "--version")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ARG:--version") {
		t.Errorf("stdout = %q, want --version to have reached the target", res.stdout)
	}
}

// Under its own name the flags are the wrapper's.
func TestEndToEndOwnFlagsUnderItsOwnName(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "exe-env-wrapper sets environment variables"},
		{name: "short help", args: []string{"-h"}, want: "exe-env-wrapper sets environment variables"},
		{name: "version", args: []string{"--version"}, want: "exe-env-wrapper "},
		// Bare, there is no tool to run and no config that was meant to
		// exist. Help beats an error about a missing yaml file.
		{name: "no arguments", args: nil, want: "exe-env-wrapper sets environment variables"},
	}

	// The published downloads carry this suffix, and it is the name a
	// first-time user actually runs.
	_, wrapper := install(t, "exe-env-wrapper-windows-amd64", "")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := run(t, wrapper, "", nil, tt.args...)
			if res.code != 0 {
				t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
			}
			if !strings.Contains(res.stdout, tt.want) {
				t.Errorf("stdout = %q, want it to contain %q", res.stdout, tt.want)
			}
		})
	}
}

// The help text has to describe what the wrapper actually does, or it is just
// decoration. These are the things a user cannot work out for themselves.
func TestUsageCoversTheEssentials(t *testing.T) {
	for _, want := range []string{
		"--wrap", "--adapt", "--setup", "--check", "--help", "--version",
		"target:", "env:", "log:",
		"${system.no_proxy}", "${VAR}",
		"exe-env-wrapper-w",
	} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage does not mention %q", want)
		}
	}
}

// Under its own name there is no tool an unrecognised argument could belong
// to, so it is a mistake rather than something to forward.
func TestEndToEndUnrecognisedOwnArgument(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "exe-env-wrapper", "")
	res := run(t, wrapper, "", nil, "--nonsense")

	if res.code == 0 {
		t.Fatalf("exit code = 0, want non-zero\nstdout: %s", res.stdout)
	}
	if !strings.Contains(res.stdout, "unrecognised argument") {
		t.Errorf("stdout = %q, want it to name the problem", res.stdout)
	}
	if !strings.Contains(res.stdout, "--wrap") {
		t.Errorf("stdout = %q, want the usage text alongside", res.stdout)
	}
}

// --wrap and --adapt without an operand must say what they need.
func TestEndToEndInstallFlagsNeedAnOperand(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "exe-env-wrapper", "")
	for _, flag := range []string{"--wrap", "--adapt"} {
		t.Run(flag, func(t *testing.T) {
			res := run(t, wrapper, "", nil, flag)
			if res.code == 0 {
				t.Fatalf("exit code = 0, want non-zero")
			}
			if !strings.Contains(res.stdout, flag+" needs") {
				t.Errorf("stdout = %q, want it to say what %s needs", res.stdout, flag)
			}
		})
	}
}
