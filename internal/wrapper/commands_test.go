package wrapper

import (
	"strings"
	"testing"
)

// Every option the parser accepts must appear in that command's help, and
// every option in the help must be accepted. A hand-written list of flags to
// check for is what let --machine, --config, --console and --force all go
// undocumented while a test claimed the usage was covered, so this walks the
// table instead.
func TestEveryOptionIsBothAcceptedAndDocumented(t *testing.T) {
	for _, cmd := range commands {
		for _, opt := range cmd.options {
			t.Run(cmd.name+" "+opt.name, func(t *testing.T) {
				var out strings.Builder
				if !renderTopic(&out, cmd.name) {
					t.Fatalf("no help topic for %q", cmd.name)
				}
				if !strings.Contains(out.String(), opt.name) {
					t.Errorf("help for %q does not mention %q:\n%s", cmd.name, opt.name, out.String())
				}
				if opt.help == "" {
					t.Errorf("option %q has no description", opt.name)
				}

				if _, err := cmd.parseOptions([]string{opt.name}); err != nil {
					t.Errorf("parseOptions rejected its own documented option %q: %v", opt.name, err)
				}
			})
		}
	}
}

// Both spellings work under the wrapper's own name. The dashed forms shipped
// in v0.3.0 and are documented, so they keep working.
func TestLookupCommandAcceptsBothForms(t *testing.T) {
	for _, name := range []string{"setup", "wrap", "adapt", "check", "help", "version"} {
		for _, spelling := range []string{name, "--" + name} {
			t.Run(spelling, func(t *testing.T) {
				cmd, ok := lookupCommand(spelling)
				if !ok {
					t.Fatalf("lookupCommand(%q) found nothing", spelling)
				}
				if cmd.name != name {
					t.Errorf("lookupCommand(%q) = %q, want %q", spelling, cmd.name, name)
				}
			})
		}
	}
	if _, ok := lookupCommand("nonsense"); ok {
		t.Error("lookupCommand accepted a word that is not a command")
	}
	if _, ok := lookupCommand("-h"); !ok {
		t.Error("lookupCommand should still accept -h")
	}
}

// An option that belongs to another command is a mistake worth reporting: it
// would otherwise be silently ignored and the user would think it applied.
func TestParseOptionsRejectsUnknownOption(t *testing.T) {
	cmd, ok := lookupCommand("setup")
	if !ok {
		t.Fatal("no setup command")
	}

	_, err := cmd.parseOptions([]string{"--force"})
	if err == nil {
		t.Fatal("parseOptions accepted --force for setup, want an error")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error %q should name the offending option", err)
	}
	if !strings.Contains(err.Error(), "--machine") {
		t.Errorf("error %q should list what setup does accept", err)
	}
}

func TestParseOptionsIgnoresOperands(t *testing.T) {
	cmd, _ := lookupCommand("wrap")
	opts, err := cmd.parseOptions([]string{"yt-dlp", "--config"})
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if !opts["--config"] {
		t.Error("--config was not recorded")
	}
	if opts["--console"] {
		t.Error("--console was recorded without being passed")
	}
}

// help with no topic lists every command, so the whole surface is reachable
// from one place.
func TestOverviewListsEveryCommand(t *testing.T) {
	var out strings.Builder
	renderOverview(&out)
	got := out.String()

	for _, cmd := range commands {
		if !strings.Contains(got, cmd.name) {
			t.Errorf("the overview does not mention %q", cmd.name)
		}
		if cmd.summary == "" {
			t.Errorf("command %q has no summary", cmd.name)
		}
	}
	if !strings.Contains(got, "help <command>") {
		t.Error("the overview does not say how to get detail on one command")
	}
}

// The config file is not a command but is the thing most needing explanation.
func TestConfigIsATopic(t *testing.T) {
	var out strings.Builder
	if !renderTopic(&out, "config") {
		t.Fatal("no help topic for config")
	}
	for _, want := range []string{"target:", "env:", "log:", "${system.no_proxy}", "${VAR}"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the config topic does not mention %q", want)
		}
	}
}

func TestRenderTopicRejectsAnUnknownTopic(t *testing.T) {
	var out strings.Builder
	if renderTopic(&out, "nonsense") {
		t.Error("renderTopic claimed to have a topic it does not have")
	}
}

// Each topic has to say something beyond repeating its own summary.
func TestEveryCommandHasDetail(t *testing.T) {
	for _, cmd := range commands {
		if len(cmd.detail) < len(cmd.summary) {
			t.Errorf("command %q has no detail beyond its summary", cmd.name)
		}
	}
}
