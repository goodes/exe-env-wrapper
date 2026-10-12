# exe-env-wrapper

A generic, config-driven wrapper executable. It sets environment variables and
then runs a target binary, passing everything else straight through.

The use it was written for: giving a single `uv tool install` launcher its own
proxy, when the thing calling it is an agent that cannot set per-call
environment variables. One wrapper build serves every tool, because it reads
the config file named after itself.

It also reads the proxy settings from the operating system, which is the part
that is hard to do otherwise: clients built on httpx honour `HTTP_PROXY` and
`HTTPS_PROXY` but never pick up the system's proxy bypass list, so internal
hosts get sent to the proxy and fail.

```powershell
exe-env-wrapper-windows-amd64.exe --setup        # once
exe-env-wrapper-windows-amd64.exe --wrap yt-dlp  # per tool
```

## Downloads

Every push builds all binaries and attaches them to the run on the
[Actions tab](https://github.com/goodes/exe-env-wrapper/actions) as an artifact
named `exe-env-wrapper-<version>`. Tagged versions also become
[releases](https://github.com/goodes/exe-env-wrapper/releases) with the binaries
attached individually.

| File | Use |
| --- | --- |
| `exe-env-wrapper-windows-amd64.exe` | Windows, console subsystem: interactive terminal use, and agents that already own a console |
| `exe-env-wrapper-w-windows-amd64.exe` | Windows, GUI subsystem: background agents, no window ever appears |
| `exe-env-wrapper-windows-arm64.exe` | Windows on ARM, console subsystem |
| `exe-env-wrapper-w-windows-arm64.exe` | Windows on ARM, GUI subsystem |
| `exe-env-wrapper-linux-amd64` | Linux |
| `exe-env-wrapper-linux-arm64` | Linux on ARM |

`SHA256SUMS` covers the whole set.

## How it works

The wrapper stands in for a tool under that tool's own name, and reads
`<its own name>.yaml` from its own directory: `yt.exe` reads `yt.yaml`,
`foo.exe` reads `foo.yaml`. One build serves every tool.

There are two ways to put it in front of a tool.

**The wrapper folder, `--wrap`.** The wrapper is installed into
`~/.local/wrapper`, which sits ahead of the tool on `PATH`. The real binary is
never touched, and the wrapper finds it at run time by looking for its own name
further along `PATH`. Nothing an installer does can undo this: `uv tool
upgrade` regenerates its launchers and `claude.exe` rewrites itself, and
neither one knows the wrapper folder exists. Use this wherever callers invoke
the tool by name.

**In place, `--adapt`.** The real binary is renamed aside with a tag of its
contents and the wrapper takes its old name. This is for callers that hard-code
an absolute path, where `PATH` is never consulted and the wrapper folder cannot
help. It is the weaker option: an installer that regenerates that binary
overwrites the wrapper and silently undoes it, so `--adapt` has to be re-run
afterwards.

Renamed to stand in for a tool, the wrapper takes no flags of its own: every
argument belongs to the target, so a wrapped tool's own `--help` still works.
Under its own name it takes the flags below.

## Installing

Once, to create the wrapper folder and put it on `PATH`:

```powershell
.\exe-env-wrapper-windows-amd64.exe --setup
```

Then per tool:

```powershell
.\exe-env-wrapper-windows-amd64.exe --wrap yt-dlp
```

That is the whole thing. The tool keeps working as before, now with the
system's proxy settings applied, and `yt-dlp` still resolves to the real
binary wherever it lives.

| Flag | |
| --- | --- |
| `--setup` | Create `~/.local/wrapper` and prepend it to your user `PATH`. `--setup --machine` writes the machine `PATH` instead, which needs administrator rights. |
| `--wrap <name>` | Wrap a tool found on `PATH`. Resolution stays dynamic, so the tool can be upgraded or moved. |
| `--wrap <path>` | Wrap exactly that binary, pinned in a config file. |
| `--adapt <path>` | Replace a binary in place. For hard-coded paths only. |
| `--check` | Report what each installed wrapper resolves to, and whether it is reachable. |
| `--help`, `--version` | Usage, and the build version. |

`--wrap` adds `--config` to write the defaults out for editing, and `--console`
to install the console build rather than the windowless one. `--adapt` adds
`--force` to replace an existing config.

### A note on PATH ordering, if a tool stays unwrapped

Windows composes `PATH` as every machine entry first, then every user entry. A
user entry therefore can never come before a machine one, whatever their order
within each list. So `--setup` puts the wrapper folder ahead of anything in
your user `PATH` — which is where `uv`-installed tools and `claude.exe` live —
but cannot get ahead of a tool in `C:\Program Files`. For those, either
`--setup --machine` (administrator rights) or `--adapt`.

`--setup` and `--check` both say which directories still precede the wrapper
folder, so this shows up when you run them rather than as a tool that
mysteriously ignores its wrapper.

## Example: adapting a launcher by hand

`--wrap` and `--adapt` do this for you. It is written out here because it shows
exactly what the wrapper needs on disk, which is worth seeing once if you are
debugging a setup or arranging one some other way.

`uv tool install` has put a launcher at
`%USERPROFILE%\.local\bin\yt-dlp.exe`. In PowerShell:

```powershell
cd $env:USERPROFILE\.local\bin

# 1. Move the real launcher aside.
Rename-Item yt-dlp.exe yt-dlp-cli.exe

# 2. Drop the wrapper in under the name callers already use.
Copy-Item ~\Downloads\exe-env-wrapper-w-windows-amd64.exe yt-dlp.exe

# 3. Tell it what to run and with what environment.
@'
target: yt-dlp-cli.exe
env:
  HTTP_PROXY: http://proxy.example:8080
  HTTPS_PROXY: http://proxy.example:8080
  NO_PROXY: ${system.no_proxy}
log: yt-dlp-wrapper.log
'@ | Set-Content -Encoding utf8 yt-dlp.yaml
```

Anything that calls `yt-dlp` now goes through the proxy, with no change at the
call site:

```powershell
yt-dlp -f bestaudio "https://example.com/watch?v=..."
```

Linux is the same shape:

```bash
cd ~/.local/bin
mv yt-dlp yt-dlp-cli
cp ~/Downloads/exe-env-wrapper-linux-amd64 yt-dlp
chmod +x yt-dlp
cat > yt-dlp.yaml <<'YAML'
target: yt-dlp-cli
env:
  HTTP_PROXY: http://proxy.example:8080
  HTTPS_PROXY: http://proxy.example:8080
YAML
```

> **Caveat, for `--adapt` only:** `uv tool upgrade` and
> `uv tool install --reinstall` regenerate the launcher, which overwrites a
> wrapper installed over it. Re-run `--adapt` afterwards; the `.yaml` file is
> left alone. This is the reason to prefer `--wrap`, which an upgrade cannot
> touch.

## Config reference

```yaml
target: yt-cli.exe          # required
env:                        # optional
  HTTP_PROXY: http://proxy.example:8080
  HTTPS_PROXY: ${HTTP_PROXY}
  NO_PROXY: localhost,127.0.0.1
  PATH: C:\extra\bin;${PATH}
log: yt-wrapper.log         # optional
```

**`target`** (optional) is the binary to run. Absolute, or relative to the
wrapper's own directory, never to the caller's working directory.

Left out, it means "run the next match for my own name on `PATH`, skipping my
own directory". That is what lets a wrapper in the wrapper folder shadow a tool
without naming where it lives, and keep working when that tool is regenerated
or moves. Directories are compared by identity rather than by spelling, so a
wrapper cannot resolve to itself, and another `exe-env-wrapper` is skipped
rather than handed off to.

With no config file at all, a wrapper applies the system proxy settings and
resolves its target this way, so installing one is a single copy. A default
that expands to nothing is left unset rather than applied, so a machine with no
proxy configured does not have an inherited `HTTP_PROXY` blanked out. On Windows a
target with no extension also matches `.exe`. Pointing it back at the wrapper is
refused rather than looped.

**`env`** (optional) sets or overrides variables. Everything not named here is
inherited unchanged. Values may refer to `${VAR}`, resolved against the
inherited environment plus any earlier entry in the same block, so
`PATH: C:\extra\bin;${PATH}` extends rather than replaces. A variable that is
not set expands to nothing, as in a shell. `$$` is a literal `$`, and so is a
bare `$` not followed by `{`. On Windows the override is case-insensitive, so
`HTTP_PROXY` replaces an inherited `http_proxy` instead of sitting next to it.

An unknown top-level key is an error rather than ignored, so a mistyped
`targett:` is reported instead of quietly doing nothing.

**`${system.*}`** reads the proxy configuration from the operating system
instead of repeating it in the file:

| Name | Value |
| --- | --- |
| `${system.http_proxy}` | the system's HTTP proxy, as a URL |
| `${system.https_proxy}` | the system's HTTPS proxy, as a URL |
| `${system.no_proxy}` | the system's proxy bypass list, comma-separated |

This exists because clients built on httpx honour `HTTP_PROXY` and
`HTTPS_PROXY` but never pick up the system's bypass list, so internal hosts get
sent to the proxy and fail. `NO_PROXY: ${system.no_proxy}` hands them the real
list without it having to be retyped and kept in step by hand.

They are ordinary values, so they compose:

```yaml
env:
  HTTP_PROXY: http://proxy.example:8080   # by hand
  NO_PROXY: ${system.no_proxy},localhost  # and from the system
```

A system name that expands to nothing leaves no stray separator behind: with
no proxy configured, `${system.no_proxy},localhost` is `localhost` rather than
`,localhost`. That applies only where a system name was used. An unset `${VAR}`
in the same position keeps its separator, as it does in a shell.

On Windows the source is the per-user WinInet configuration, the same settings
Windows shows under Settings, Network & internet, Proxy. Windows stores them in
a shape no client reads directly, so they are translated: the bypass list's
semicolons become commas, its `<local>` token becomes
`localhost,127.0.0.1,::1`, and a proxy stored either as a bare `host:port` or
as a per-scheme `http=...;https=...` list gains the `http://` prefix the
environment variables are read as. A `socks=` entry is ignored.

With the system set to use no proxy, the two proxy values come back empty but
the bypass list is still returned, which is what makes "proxy by hand, bypass
list from the system" work. Nothing configured expands to empty, like an unset
variable. A misspelled name such as `${system.no_proxyy}` fails the run rather
than expanding to empty, since a proxy setting that silently does nothing is
the problem this is here to avoid.

On Linux there is no system proxy store, so the three names resolve from the
inherited environment, matching either case, keeping one config file portable.

**`log`** (optional) is a file the wrapper appends its own failures to, resolved
beside the wrapper if relative. Worth setting for the windowless build, whose
stderr usually goes nowhere. It only ever records the wrapper's own problems;
the target's output is never touched. To catch a config file so broken that
`log` itself cannot be read, set `EXE_ENV_WRAPPER_LOG` in the environment
instead.

A `.yml` extension works too, with `.yaml` preferred when both exist.

## Behaviour

- All command-line arguments reach the target untouched, with the caller's
  quoting preserved. On Windows the raw command line is forwarded rather than
  re-escaped from a parse of it.
- stdin, stdout and stderr are passed through.
- The target's exit code is the wrapper's exit code. `125` means the wrapper
  itself could not start the target: bad config, target missing, or nothing of
  its own name further along `PATH`. Those failures print to stderr and go to
  `log` if it is set. A missing config file is not one of them: a wrapper
  without one runs on the built-in defaults.
- Under its own name the exit status is the command's own: `0` for success, `1`
  for a failed `--setup`, `--wrap` or `--adapt`, or for a `--check` that found
  a problem, and `2` for an argument it does not recognise.
- An unknown key in the config file is an error rather than ignored, so a
  mistyped `targett:` is reported instead of quietly doing nothing.
- On Linux the wrapper `exec`s in place, replacing its own process, so nothing
  extra sits between the caller and the target.

## Windows console handling

A console window must not flash when a background agent spawns the wrapper, and
interactive use from a terminal must still show output. No single executable
does both, so there are two builds from the same source:

- **`exe-env-wrapper.exe`** is a console-subsystem binary. Run from a terminal
  it behaves exactly like the tool it wraps: `cmd` waits for it, output and
  progress bars land in the console the child inherits. Spawned from a parent
  that has no console, Windows gives it one, which is the window flash.
- **`exe-env-wrapper-w.exe`** is linked with `-H windowsgui`, so Windows never
  gives it a console, and it spawns the target with `CREATE_NO_WINDOW` so the
  console child does not get a fresh window either. The standard handles are
  passed through, so redirected output still reaches whoever asked for it. Run
  from `cmd` interactively, a GUI-subsystem binary is not waited for, so use the
  console build there.

Pick the windowless build for agents, the console build for yourself.

## Building

Go 1.22 or newer, no cgo, no runtime to install alongside.

```bash
go test ./...
scripts/build.sh v0.1.0   # everything into dist/
```

One target at a time:

```bash
# Linux
go build -o exe-env-wrapper ./cmd/exe-env-wrapper

# Windows, console
GOOS=windows GOARCH=amd64 go build -o exe-env-wrapper.exe ./cmd/exe-env-wrapper

# Windows, windowless
GOOS=windows GOARCH=amd64 go build -tags windowsgui \
  -ldflags "-H windowsgui" -o exe-env-wrapper-w.exe ./cmd/exe-env-wrapper
```

The `windowsgui` build tag and the `-H windowsgui` link flag go together: the
tag turns on `CREATE_NO_WINDOW` for the child, the flag puts the wrapper itself
in the GUI subsystem.

### Installing Go on Windows

```powershell
winget install GoLang.Go
```

Or the MSI from [go.dev/dl](https://go.dev/dl/). Open a new terminal afterwards
so `go` is on `PATH`.

## Not here yet

Signed releases. Windows Smart App Control judges a binary on its signature
and reputation, and blocks an unsigned one whether it was downloaded or built
locally, with no way to allow it. Until the released binaries are signed, a
machine with Smart App Control enabled cannot run them. Azure Trusted Signing
or the SignPath Foundation would both fix this from CI.


A Python implementation of the same behaviour, built with PyInstaller
(`--onefile`, plus `--noconsole` for the windowless variant), is in the design
but not written. The Go build covers the use case on its own.
