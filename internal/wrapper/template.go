package wrapper

import (
	"fmt"
	"strings"
)

// configTemplate is the config file wrap --config and adapt write.
//
// Everything is present, and everything but the env block is commented out, so
// the file both does the useful thing by default and shows what else can be
// changed. Someone opening it should not have to go looking for the reference
// to find out what a wrapper config can contain.
//
// target is live when there is one to pin, and commented when resolution is
// meant to stay dynamic. Writing it commented in the pinned case would leave
// the wrapper resolving from PATH and ignoring the file it was just given.
func configTemplate(name, target string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s.yaml — read by the wrapper named %s, sitting beside it.\n", name, name)
	b.WriteString("# Full reference: exe-env-wrapper help config\n\n")

	if target != "" {
		fmt.Fprintf(&b, "target: %s\n", target)
		b.WriteString("#   What to run. This one is pinned to the path above. Absolute, or\n")
		b.WriteString("#   relative to this file's directory. Removed altogether, it means \"the\n")
		b.WriteString("#   next match for my own name on PATH\".\n")
	} else {
		fmt.Fprintf(&b, "# target: %s\n", executableName(name))
		b.WriteString("#   What to run. Absolute, or relative to this file's directory. Left out,\n")
		b.WriteString("#   as it is here, it means \"the next match for my own name on PATH\", so\n")
		b.WriteString("#   the real tool can be upgraded or moved without touching this file.\n")
	}

	b.WriteString("\nenv:\n")
	b.WriteString("  HTTP_PROXY: ${system.http_proxy}\n")
	b.WriteString("  HTTPS_PROXY: ${system.https_proxy}\n")
	b.WriteString("  NO_PROXY: ${system.no_proxy}\n")
	b.WriteString("#   Variables to set or override; anything not named here is inherited\n")
	b.WriteString("#   unchanged. ${system.*} comes from the operating system, ${VAR} from the\n")
	b.WriteString("#   inherited environment. NO_PROXY is the one that matters: clients built\n")
	b.WriteString("#   on httpx never pick up the system's bypass list on their own. Add your\n")
	b.WriteString("#   own entries alongside it:\n")
	b.WriteString("#     NO_PROXY: ${system.no_proxy},internal.example\n")

	fmt.Fprintf(&b, "\n# log: %s.log\n", name)
	b.WriteString("#   The wrapper's own failures, and only those. Staying empty means nothing\n")
	b.WriteString("#   has gone wrong.\n")

	fmt.Fprintf(&b, "\n# trace: %s-trace.log\n", name)
	b.WriteString("# trace_level: 1\n")
	b.WriteString("#   A transcript, and how much goes in it: 1 the command, 2 also the\n")
	b.WriteString("#   target's streams, 3 also every variable the wrapper changed with its\n")
	b.WriteString("#   value before and after. Only 2 and above puts the wrapper in the data\n")
	b.WriteString("#   path, so 1 costs nothing.\n")

	return b.String()
}
