package wrapper

import (
	"fmt"
	"io"
	"strings"
)

// option is a flag belonging to one command.
type option struct {
	name string
	help string
}

// command is one of the wrapper's own commands, available only under the
// wrapper's own name.
//
// The table is the single source of truth: the parser accepts what is listed
// here and the help text is rendered from it. Writing the help by hand is what
// let --machine, --config, --console and --force all ship undocumented.
type command struct {
	name string
	// operand is how the argument is shown in help, empty if it takes none.
	operand string
	// summary is the one-line form, for the overview.
	summary string
	options []option
	// detail is what `help <command>` prints.
	detail string
}

var commands = []command{
	{
		name:    "setup",
		summary: "Create the wrapper folder and put it on PATH.",
		options: []option{{
			name: "--machine",
			help: "Write the machine PATH rather than your own user PATH. Needs\n" +
				"administrator rights. This is the only way to get ahead of a tool\n" +
				"installed in a machine PATH directory, such as C:\\Program Files.",
		}},
		detail: `Creates ~/.local/wrapper and moves it to the front of PATH.

Run this once, before wrapping anything. Wrappers live in that folder, and a
folder that is not on PATH ahead of a tool is never reached, so a wrapper in it
would silently do nothing.

Windows composes PATH as every machine entry first, then every user entry. A
user entry therefore can never come before a machine one, whatever their order
within each list. Putting the wrapper folder at the front of your user PATH
gets it ahead of ~/.local/bin, which is where uv-installed tools and claude.exe
live, but not ahead of anything in C:\Program Files. For those, use --machine,
or adapt them in place instead.

setup reports which directories still precede the wrapper folder, so a tool
that cannot be wrapped from there is named at the time rather than discovered
later. check reports the same thing.`,
	},
	{
		name:    "wrap",
		operand: "<name|path>",
		summary: "Install a wrapper for a tool, leaving the original alone.",
		options: []option{
			{
				name: "--config",
				help: "Also write a config file holding the defaults, as a starting\n" +
					"point for editing. Without it no config file is written, because\n" +
					"a wrapper without one already applies those defaults.",
			},
			{
				name: "--windowless",
				help: "Install the windowless build instead of this one. Only wanted\n" +
					"under a host that has no console of its own and does not pass\n" +
					"CREATE_NO_WINDOW, where a console wrapper would make Windows\n" +
					"allocate a console and flash a window. Never use it for a tool\n" +
					"you run from a shell: a shell decides whether to wait from the\n" +
					"subsystem alone, so it would not wait, would show no output, and\n" +
					"would report the wrong exit status. Needs the -w build in this\n" +
					"same directory.",
			},
		},
		detail: `Installs a wrapper into the wrapper folder under the tool's own name. The real
binary is never renamed, moved or modified.

The console build is installed by default, and is what you want for a
command-line tool. Its callers — a shell, or an agent running it as a shell —
already own a console, which the wrapper and the tool inherit, so nothing is
allocated and no window appears.

A bare name is resolved on PATH, and resolution stays dynamic: the wrapper
finds the real tool at run time by looking for its own name further along PATH.
That is the point of wrapping rather than adapting. An installer may regenerate
the tool, as uv tool upgrade does, or move it to another directory, and the
wrapper keeps working.

A path pins exactly that one file, recorded in a config file because it cannot
be rediscovered.

Only .exe files can be wrapped. A .cmd or .bat would have to be run through
cmd.exe, which gives up the raw command-line pass-through the wrapper exists to
preserve.

An existing config file is left alone, never overwritten.`,
	},
	{
		name:    "adapt",
		operand: "<path>",
		summary: "Replace a binary in place. For hard-coded paths only.",
		options: []option{
			{
				name: "--force",
				help: "Replace an existing config file instead of leaving it alone.",
			},
			{
				name: "--windowless",
				help: "Install the windowless build instead of this one. See\n" +
					"\"help wrap\": it is almost never what you want for a\n" +
					"command-line tool.",
			},
		},
		detail: `Renames a binary aside, tagged with a hash of its contents, and installs the
wrapper under the name callers already use.

This is for callers that hard-code an absolute path, where PATH is never
consulted and so the wrapper folder cannot help. Prefer wrap wherever callers
invoke the tool by name: an installer that regenerates the binary overwrites an
adapted wrapper and silently undoes it, which has to be noticed and re-run,
whereas nothing an installer does touches the wrapper folder.

The argument must be a path, or a bare name in the wrapper's own directory.
PATH is deliberately not searched: a name resolved from PATH would be the wrong
file to replace.

Re-running on the same binary does nothing, because the hashed name already
holds those contents. A binary that is already a wrapper is refused rather than
adapted again.`,
	},
	{
		name:    "check",
		summary: "Report what each installed wrapper resolves to.",
		detail: `Lists every wrapper in the wrapper folder with the config it uses and the
binary it resolves to, and says whether the folder is on PATH at all.

Every way this breaks is invisible in normal use: a wrapper that stopped being
reached because PATH changed, one whose tool has been uninstalled, one whose
config stopped parsing. The exit status is non-zero if anything is wrong, so it
is usable from a script.`,
	},
	{
		name:    "help",
		operand: "[command]",
		summary: "This text, or detail on one command.",
		detail: `With no argument, lists every command. With one, prints that command's detail
and its options.

help config explains the config file.`,
	},
	{
		name:    "version",
		summary: "The build version.",
		detail:  `Prints the version this binary was built from.`,
	},
}

// extraTopics are help topics that are not commands.
var extraTopics = map[string]string{
	"config": `A wrapper reads <its own name>.yaml from its own directory: yt.exe reads
yt.yaml, foo.exe reads foo.yaml. A .yml extension works too.

  target: yt-cli.exe        Optional. Absolute, or relative to the wrapper's
                            own directory, never to the caller's working
                            directory. Omitted, it means "run the next match
                            for my own name on PATH, skipping my own
                            directory", which is how a wrapper in the wrapper
                            folder shadows a tool without naming its location.
  env:                      Optional. Variables to set or override. Anything
    HTTP_PROXY: http://p:1  not named here is inherited unchanged.
  log: wrapper.log          Optional. A file the wrapper appends its own
                            failures to, and only those. A run that worked
                            writes nothing, so the file staying empty means
                            nothing went wrong.
  trace: wrapper-trace.log  Optional. Where a transcript goes.
  trace_level: 1            Optional, 1 when trace is set. How much goes in
                            it.

Values may refer to ${VAR}, resolved against the inherited environment plus any
earlier entry in the same block, so PATH: C:\extra\bin;${PATH} extends rather
than replaces. A variable that is not set expands to nothing, as in a shell.
$$ is a literal $, and so is a bare $ not followed by {.

Three names come from the operating system instead:

  ${system.http_proxy}      The system's HTTP proxy, as a URL.
  ${system.https_proxy}     The system's HTTPS proxy, as a URL.
  ${system.no_proxy}        The system's proxy bypass list, comma-separated.

The bypass list is the one that matters: clients built on httpx honour
HTTP_PROXY and HTTPS_PROXY but never pick up the system's bypass list, so
internal hosts get sent to the proxy and fail. A misspelled system name fails
the run rather than expanding to nothing, since a proxy setting that silently
does nothing is the problem this avoids. A system name that expands to nothing
leaves no stray separator behind.

With no config file at all, a wrapper applies the system proxy settings and
resolves its target from PATH, so installing one is a single copy. A default
that expands to nothing is left unset rather than applied, so a machine with no
proxy configured does not have an inherited HTTP_PROXY blanked out.

An unknown key is an error rather than ignored, so a mistyped targett: is
reported instead of quietly doing nothing.

The trace levels, each including the ones below it:

  1   The resolved target and the command line.
  2   Also the target's stdin, stdout and stderr, marked << >> and !!. For a
      stdio tool, an MCP server among them, that is the whole conversation.
  3   Also every variable the wrapper changed, with the value it had before
      and the value it was given, verbatim. This is what answers "why is this
      variable not what I configured", including the variables the wrapper
      declined to set because their value came to nothing.

Level 1 is the default because it is free: only level 2 and above has to copy
the streams through the wrapper. Below that the target inherits the standard
handles directly, and on Linux the wrapper is replaced by the target
altogether, so nothing sits between the caller and the tool. At level 2 and
above the real stream is always written before the transcript, so a slow trace
cannot delay a protocol message, but it is still something to turn on to
investigate rather than leave running.

A wrapper installed by wrap has no config file, so it has nowhere to put these.
Two environment variables say the same thing from outside, which also lets a
wrapper be watched without editing the installation being watched:

  EXE_ENV_WRAPPER_TRACE=/path/to/trace.log
  EXE_ENV_WRAPPER_TRACE_LEVEL=3

They win over the config file, since they are reached for when something is
already misbehaving. Nothing about a trace can fail the run: a level with
nowhere to write, an unreadable level, or a file that cannot be opened is
reported, and the tool runs anyway. Losing a debugging aid must not take the
tool down with it.`,
}

// lookupCommand finds a command by name, with or without the leading dashes.
//
// Both spellings work because the dashed forms shipped in v0.3.0 and are
// documented. Under the wrapper's own name there is no tool the arguments could
// belong to, so a bare word is unambiguous.
func lookupCommand(arg string) (*command, bool) {
	name := strings.TrimPrefix(arg, "--")
	if arg == "-h" {
		name = "help"
	}
	for i := range commands {
		if commands[i].name == name {
			return &commands[i], true
		}
	}
	return nil, false
}

// parseOptions collects the options present in args, rejecting any that belong
// to a different command.
//
// Rejecting rather than ignoring matters: an ignored --force would leave the
// user believing a config had been replaced when it had not.
func (c *command) parseOptions(args []string) (map[string]bool, error) {
	found := make(map[string]bool, len(c.options))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue // An operand, not an option.
		}
		known := false
		for _, opt := range c.options {
			if arg == opt.name {
				found[arg] = true
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("%s does not take %s%s", c.name, arg, c.acceptedOptions())
		}
	}
	return found, nil
}

// acceptedOptions renders what a command does take, for an error message.
func (c *command) acceptedOptions() string {
	if len(c.options) == 0 {
		return "; it takes no options"
	}
	names := make([]string, 0, len(c.options))
	for _, opt := range c.options {
		names = append(names, opt.name)
	}
	return "; it takes " + strings.Join(names, " and ")
}

// invocation is how a command is written, for help.
func (c *command) invocation() string {
	parts := []string{c.name}
	if c.operand != "" {
		parts = append(parts, c.operand)
	}
	for _, opt := range c.options {
		parts = append(parts, "["+opt.name+"]")
	}
	return strings.Join(parts, " ")
}

const overviewIntro = `exe-env-wrapper sets environment variables from a YAML file named after
itself, then runs a target binary with every argument passed straight through.

Renamed to stand in for a tool it takes no arguments of its own: they all
belong to that tool, so its own --help keeps working. Under its own name, as
now, it takes the commands below.

`

const overviewOutro = `
Dashes are optional: setup and --setup are the same. Run "help <command>" for
detail, or "help config" for the config file.
`

// renderOverview lists every command.
func renderOverview(w io.Writer) {
	fmt.Fprint(w, overviewIntro)
	width := 0
	for _, cmd := range commands {
		if n := len(cmd.name) + 1 + len(cmd.operand); n > width {
			width = n
		}
	}
	for _, cmd := range commands {
		left := cmd.name
		if cmd.operand != "" {
			left += " " + cmd.operand
		}
		fmt.Fprintf(w, "  %-*s  %s\n", width, left, cmd.summary)
	}
	fmt.Fprint(w, overviewOutro)
}

// renderTopic prints one topic, reporting whether it exists.
func renderTopic(w io.Writer, topic string) bool {
	if text, ok := extraTopics[strings.TrimPrefix(topic, "--")]; ok {
		fmt.Fprintf(w, "%s\n", text)
		return true
	}
	cmd, ok := lookupCommand(topic)
	if !ok {
		return false
	}

	fmt.Fprintf(w, "  %s\n\n%s\n", cmd.invocation(), cmd.detail)
	if len(cmd.options) > 0 {
		fmt.Fprint(w, "\nOptions:\n")
		for _, opt := range cmd.options {
			fmt.Fprintf(w, "\n  %s\n", opt.name)
			for _, line := range strings.Split(opt.help, "\n") {
				fmt.Fprintf(w, "      %s\n", line)
			}
		}
	}
	return true
}

// topicNames lists everything help accepts, for an unknown-topic message.
func topicNames() string {
	names := make([]string, 0, len(commands)+len(extraTopics))
	for _, cmd := range commands {
		names = append(names, cmd.name)
	}
	for name := range extraTopics {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
