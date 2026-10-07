# exe-env-wrapper

A generic, config-driven wrapper executable. It sets environment variables and
then runs a target binary, passing everything else straight through.

The use it was written for: giving a single `uv tool install` launcher its own
proxy, when the thing calling it is an agent that cannot set per-call
environment variables. One wrapper build serves every tool, because it reads
the config file named after itself.

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
| `exe-env-wrapper-linux-amd64` | Linux |

`arm64` builds of each are published too. `SHA256SUMS` covers the whole set.

## How it works

1. Rename the original launcher, e.g. `yt.exe` to `yt-cli.exe`. This is safe for
   uv launchers: they locate their environment by where they sit, not by what
   they are called.
2. Copy the wrapper in beside it under the original name, `yt.exe`.
3. The wrapper reads `<its own name>.yaml` from its own directory. `yt.exe`
   reads `yt.yaml`, `foo.exe` reads `foo.yaml`.

The wrapper takes no flags of its own. Every argument belongs to the target.

## Example: yt-dlp through a proxy

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
  NO_PROXY: localhost,127.0.0.1
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

> **Caveat:** `uv tool upgrade` and `uv tool install --reinstall` regenerate the
> launcher, which overwrites the wrapper at the original name. Redo steps 1 and
> 2 afterwards. The `.yaml` file is left alone.

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

**`target`** (required) is the binary to run. Absolute, or relative to the
wrapper's own directory, never to the caller's working directory. On Windows a
target with no extension also matches `.exe`. Pointing it back at the wrapper is
refused rather than looped.

**`env`** (optional) sets or overrides variables. Everything not named here is
inherited unchanged. Values may refer to `${VAR}`, resolved against the
inherited environment plus any earlier entry in the same block, so
`PATH: C:\extra\bin;${PATH}` extends rather than replaces. A variable that is
not set expands to nothing, as in a shell. `$$` is a literal `$`, and so is a
bare `$` not followed by `{`. On Windows the override is case-insensitive, so
`HTTP_PROXY` replaces an inherited `http_proxy` instead of sitting next to it.

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
  itself could not start the target: no config, bad config, or target missing.
  Those failures print to stderr and go to `log` if it is set.
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

A Python implementation of the same behaviour, built with PyInstaller
(`--onefile`, plus `--noconsole` for the windowless variant), is in the design
but not written. The Go build covers the use case on its own.
