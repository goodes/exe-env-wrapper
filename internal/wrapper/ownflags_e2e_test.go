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

// Every command in the table must be dispatched. The switch has a fallthrough
// that reports "not implemented", and a command added to the table without a
// case would otherwise reach it only when a user tried it.
func TestEveryCommandIsDispatched(t *testing.T) {
	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			var out strings.Builder
			// help is the one command safe to run with no side effects and no
			// dependence on the real environment, so commands are probed via
			// their help topic. Dispatch itself is covered by asking for a
			// command that exists and checking it is not reported missing.
			if code := runCommandMode([]string{"help", cmd.name}, &out); code != 0 {
				t.Fatalf("help %s exited %d:\n%s", cmd.name, code, out.String())
			}
			if strings.Contains(out.String(), "not implemented") {
				t.Errorf("command %q is in the table but not dispatched", cmd.name)
			}
		})
	}
}

// The dash-less and dashed spellings both reach the same command, end to end.
func TestEndToEndBareAndDashedSpellings(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "exe-env-wrapper", "")
	for _, args := range [][]string{
		{"version"}, {"--version"},
		{"help"}, {"--help"},
		{"help", "setup"}, {"--help", "setup"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			res := run(t, wrapper, "", nil, args...)
			if res.code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout: %s", res.code, res.stdout)
			}
		})
	}
}

// The question that prompted all of this: the options have to be reachable
// from the tool, not only from the README.
func TestEndToEndHelpOnACommandNamesItsOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "exe-env-wrapper", "")

	// Walked from the table rather than listed here. A hand-written list is
	// what let the options go undocumented in the first place, and then went
	// stale again when --console became --windowless.
	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			res := run(t, wrapper, "", nil, "help", cmd.name)
			if res.code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout: %s", res.code, res.stdout)
			}
			for _, opt := range cmd.options {
				if !strings.Contains(res.stdout, opt.name) {
					t.Errorf("help %s does not mention %q:\n%s", cmd.name, opt.name, res.stdout)
				}
			}
		})
	}

	// The config topic is not a command, so it is checked by content.
	t.Run("config", func(t *testing.T) {
		res := run(t, wrapper, "", nil, "help", "config")
		if res.code != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout: %s", res.code, res.stdout)
		}
		for _, want := range []string{"target:", "env:", "log:", "trace:", "${system.no_proxy}"} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("help config does not mention %q:\n%s", want, res.stdout)
			}
		}
	})
}

// A bare word under an adapted name belongs to the tool. This is the risk the
// dash-less spellings introduce: "yt-dlp setup" must reach yt-dlp.
func TestEndToEndBareCommandWordReachesTheTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "yt", "target: yt-cli"+exeSuffix()+"\n")
	for _, word := range []string{"setup", "wrap", "adapt", "check", "help", "version"} {
		t.Run(word, func(t *testing.T) {
			res := run(t, wrapper, "", nil, word)
			if res.code != 0 {
				t.Fatalf("exit code = %d, want 0\nstderr: %s", res.code, res.stderr)
			}
			if !strings.Contains(res.stdout, "ARG:"+word) {
				t.Errorf("stdout = %q, want %q to have reached the target", res.stdout, word)
			}
		})
	}
}

// Under its own name there is no tool an unrecognised command could belong
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
	if !strings.Contains(res.stdout, "unrecognised command") {
		t.Errorf("stdout = %q, want it to name the problem", res.stdout)
	}
	if !strings.Contains(res.stdout, "wrap <name|path>") {
		t.Errorf("stdout = %q, want the command list alongside", res.stdout)
	}
	if !strings.Contains(res.stdout, `help <command>`) {
		t.Errorf("stdout = %q, want it to point at per-command help", res.stdout)
	}
}

// --wrap and --adapt without an operand must say what they need.
func TestEndToEndInstallFlagsNeedAnOperand(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}

	_, wrapper := install(t, "exe-env-wrapper", "")
	for _, name := range []string{"wrap", "adapt"} {
		for _, spelling := range []string{name, "--" + name} {
			t.Run(spelling, func(t *testing.T) {
				res := run(t, wrapper, "", nil, spelling)
				if res.code == 0 {
					t.Fatalf("exit code = 0, want non-zero")
				}
				if !strings.Contains(res.stdout, name+" needs") {
					t.Errorf("stdout = %q, want it to say what %s needs", res.stdout, name)
				}
				if !strings.Contains(res.stdout, "help "+name) {
					t.Errorf("stdout = %q, want it to point at help %s", res.stdout, name)
				}
			})
		}
	}
}
