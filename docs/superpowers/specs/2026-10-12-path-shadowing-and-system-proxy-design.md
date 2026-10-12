# Design: system proxy expansion, PATH shadowing, and self-installation

Date: 2026-10-12
Status: implemented. The proxy work shipped in v0.2.0; the PATH-shadowing and
self-installation work followed on the feat/path-shadowing branch.

Changes made while implementing, beyond what is written below:

- `.exe`-only wrapping, install-time-only PATH checking, the two `--wrap`
  argument forms and `--adapt` never consulting PATH were all decided after
  the first draft and are reflected in the text.
- `--setup` writes the user PATH by default, with `--machine` for the machine
  one. Windows composes PATH as machine entries then user entries, so a user
  entry can never precede a machine one; the spec had assumed prepending was
  enough.
- The built-in defaults skip any variable that expands to nothing, which the
  spec had not considered. Without it, a machine with no proxy configured
  would have had an inherited HTTP_PROXY blanked out by the defaults.
- A system name expanding to nothing no longer leaves a stray list separator.
  End-to-end testing found `NO_PROXY=,localhost`, which the unit tests had
  missed by only ever supplying a populated list.
- Unknown arguments under the wrapper's own name exit 2 with usage, rather
  than falling through to normal wrapper behaviour.

Later, in v0.3.1 and v0.3.2:

- The commands and their options became one table that both the parser and the
  help text read from. The hand-written help had listed the commands and none
  of their options, so `--machine`, `--config`, `--console` and `--force` all
  shipped undocumented while a test claimed the usage was covered.
- Dashes became optional, since under the wrapper's own name a bare word is
  unambiguous. `Run` split into the two programs it always was, chosen once by
  the name at startup.
- The windowless build stopped being the default. It was chosen on the premise
  that background agents are the main caller; the actual callers are shells and
  agents running tools as shells, which own a console that a console-subsystem
  child inherits, so nothing is allocated and no window appears. A shell does
  not wait for a GUI-subsystem binary at all, so the windowless default would
  have delivered no output and the wrong exit status. It is now `--windowless`,
  for a host with no console that does not pass CREATE_NO_WINDOW.
- `log` records the command line of every run, not only failures, with the raw
  command line on Windows so the caller's own quoting is preserved.
- `trace` was added: an opt-in transcript of the target's stdin, stdout and
  stderr. It is opt-in because it puts the wrapper in the data path, which the
  default avoids entirely — the target inherits the handles directly, and on
  Unix the wrapper is replaced by the target. The real stream is always written
  before the transcript so a slow trace cannot delay a protocol message, and
  nothing waits for a newline. Driven by stdio MCP servers being a main use
  case, which also added round-trip tests that fail on buffering rather than
  hanging.

## Purpose

Two problems, one wrapper.

**Problem 1 — the bypass list never arrives.** A tool put on a proxy through
`HTTP_PROXY`/`HTTPS_PROXY` also needs to know what *not* to proxy. Clients
built on httpx honour the proxy variables but do not pick up the operating
system's proxy bypass list, so internal hosts get sent to the proxy and fail.
Today the only fix is to retype the bypass list into every wrapper config by
hand and keep it in step with the system settings.

**Problem 2 — installers overwrite the wrapper.** The wrapper currently works
by replacing a launcher at its own path. `uv tool upgrade`,
`uv tool install --reinstall`, and self-updating binaries such as `claude.exe`
all regenerate that file, silently removing the wrapper and with it the
environment it was applying. The README documents this as a caveat and tells
the user to redo the installation by hand.

This design fixes the first by reading the proxy settings from the OS, and the
second by moving the wrapper off the contested path entirely.

## Scope

In scope:

1. `${system.*}` expansion, reading the Windows WinInet proxy settings.
2. Optional `target`, with runtime resolution to the next match on `PATH`.
3. Self-installation: `--wrap`, `--adapt`, `--setup`, `--check`, `--help`,
   `--version`.
4. PATH ordering verification, at install time only.

Out of scope, deliberately:

- PAC / `AutoConfigURL` evaluation. The bypass list in a PAC file is
  JavaScript evaluated per URL, not a static list, so there is nothing to
  translate. The user has confirmed their settings are WinInet, not PAC.
- Reading GNOME/macOS proxy settings. The non-Windows builds fall back to the
  inherited environment (see below), which is sufficient for the Linux use case.
- `socks=` proxies in `ProxyServer`. Parsed over and ignored.
- Wrapping anything but an `.exe`. A `.cmd` or `.bat` target would have to be
  spawned through `cmd.exe /c`, which breaks both the raw command-line
  pass-through on Windows and the `exec`-in-place behaviour on Linux. Both
  install commands accept `.exe` only and say so when refusing.
- An uninstall command. Removing a wrapped tool is deleting one file from the
  wrapper folder; a command to do that earns its place only if asked for.

## Feature 1: `${system.*}` expansion

### Syntax

Three new names inside the existing `${...}` expansion:

```yaml
env:
  HTTP_PROXY: ${system.http_proxy}
  HTTPS_PROXY: ${system.https_proxy}
  NO_PROXY: ${system.no_proxy}
```

They compose with literal text and with each other, because they go through
the same expansion pass as everything else:

```yaml
env:
  NO_PROXY: ${system.no_proxy},localhost,.internal
```

The `system.` namespace cannot collide with a real environment variable,
because environment variable names do not contain dots.

Everything that works today keeps working unchanged. This is a new value form,
not a replacement for manual configuration.

### Unknown names are an error

`${system.foo}` fails the run with a clear message naming the three valid
options. This differs from an unset `${VAR}`, which expands to empty by design
— an unset variable is a normal condition, whereas a misspelled `system.` name
is a config mistake that would otherwise manifest as a silently empty proxy
setting.

This requires `Expand`'s lookup callback to gain an `error` return:

```go
func Expand(s string, lookup func(string) (string, error)) (string, error)
```

### Source: Windows WinInet

Read from `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`:

| Value | Type | Meaning |
|---|---|---|
| `ProxyEnable` | `DWORD` | 0 = direct, 1 = use `ProxyServer` |
| `ProxyServer` | `REG_SZ` | the proxy, in one of two forms |
| `ProxyOverride` | `REG_SZ` | the bypass list |

The read happens **lazily**: the registry is only opened if a `${system.*}`
name actually appears in the config, and at most once per run regardless of how
many of the three are referenced.

### Translation rules

`ProxyOverride` → `no_proxy`:

- Split on `;`, trim each entry, drop empty entries, join with `,`.
  `NO_PROXY` consumers expect commas; WinInet stores semicolons.
- The `<local>` token means "bypass for addresses without a dot" and has no
  `NO_PROXY` equivalent. It becomes `localhost,127.0.0.1,::1`.
- Order is preserved, and duplicates produced by the `<local>` expansion are
  removed.

`ProxyServer` → `http_proxy` / `https_proxy`:

- Two stored forms must both be handled:
  - bare `host:port` — applies to all schemes
  - per-scheme `http=host:port;https=host:port;ftp=...` — pick the matching key
- A value with no scheme gets `http://` prefixed, since proxy environment
  variables are consumed as URLs. A value that already has a scheme is left
  alone.
- `socks=` and any other unrecognised scheme key is ignored.

`ProxyEnable` semantics:

- `ProxyEnable = 0` → `http_proxy` and `https_proxy` expand to empty.
- `ProxyEnable = 0` → **`no_proxy` still returns the bypass list.** This is
  deliberate and is the main use case: specifying the proxy by hand while
  taking only the bypass list from the system. The bypass list is inert when no
  proxy is set, so returning it costs nothing.

Nothing configured, or the key missing entirely, expands to empty without
error — the same rule as an unset variable.

### Non-Windows behaviour

There is no system proxy store to read. `${system.http_proxy}`,
`${system.https_proxy}` and `${system.no_proxy}` resolve from the inherited
environment, matching either case (`no_proxy` or `NO_PROXY`). This makes the
names harmless and predictable on Linux rather than an error, and keeps one
config file portable across platforms.

### Files

- `sysproxy.go` — the translation, as a pure function from raw registry
  strings to the three resolved values. All the logic lives here and is
  testable on any platform.
- `sysproxy_windows.go` — the registry read. Build-tagged.
- `sysproxy_other.go` — the environment fallback. Build-tagged.

### Dependency

`golang.org/x/sys/windows/registry`. Pure Go, no cgo, no transitive
dependencies, so the README's "no cgo, no runtime to install alongside" stays
true. The zero-dependency alternative is hand-rolled `advapi32` syscalls, which
is roughly 60 lines of unsafe pointer work to save one well-known module.

## Feature 2: target resolution

### `target` becomes optional

| Config | Resolution |
|---|---|
| `target:` present | as today: absolute, or relative to the wrapper's directory |
| `target:` absent | the next match for the wrapper's own base name on `PATH`, excluding the wrapper's own directory |

The second mode is what makes PATH shadowing work. A wrapper named
`yt-dlp.exe` sitting in the wrapper folder finds the real `yt-dlp.exe`
wherever it currently lives — so an upgrade that regenerates the real binary,
or even moves it to a different directory, changes nothing.

### Next-on-PATH algorithm

1. Take the wrapper's own base name (`yt-dlp` from `yt-dlp.exe`).
2. Walk `PATH` entries in order.
3. Skip any entry that resolves to the wrapper's own directory. Compared by
   **file identity**, not string equality: `PATH` routinely lists the same
   directory twice under different spellings, and a string comparison would
   let the wrapper find itself.
4. In each remaining entry, look for `<base name>.exe`. Only `.exe` is
   considered; see Scope.
5. Skip any candidate that is itself an exe-env-wrapper build, detected by the
   marker below. Without this, two wrappers on `PATH` could hand off to each
   other indefinitely.
6. The first surviving candidate is the target.

If no candidate is found, the run fails with exit 125 and a message naming the
base name searched for and the `PATH` entries examined.

### Unknown config keys become an error

Today a config is decoded with unknown keys ignored, and a missing `target` is
a hard error. Once a missing `target` means "resolve from `PATH`", a typo such
as `targett:` would silently change behaviour from "run this binary" to "search
`PATH`" — a confusing failure.

The YAML decoder therefore switches to known-fields-only, so `targett:` is
rejected by name. This is a small hardening of existing behaviour, included
because this change is what makes it necessary.

### The wrapper marker

A distinctive constant string compiled into every build, used to answer "is
this file an exe-env-wrapper?" for two callers: next-on-PATH resolution, and
`--adapt` refusing to adapt a wrapper.

Detection is a streaming scan of the candidate file's bytes for the marker,
with overlap handling across read boundaries so a marker split across two
buffers is still found. Streaming matters: `claude.exe` in `~/.local/bin` is
237 MB.

## Feature 3: self-installation

### The name gate

The wrapper interprets its own flags only when its base name still carries the
`exe-env-wrapper` prefix. Under any other name every argument belongs to the
target, as today.

The prefix match — rather than an exact name — is required because the
published downloads are named `exe-env-wrapper-windows-amd64.exe` and
`exe-env-wrapper-w-windows-amd64.exe`, and those are the names the user will
actually invoke straight out of a downloads folder.

**The behaviour this must not break:** under an adapted name, `--help` and
`--version` belong to the target. `yt-dlp --help` must print yt-dlp's help and
`uv --version` must report uv's version. This is the first thing to write a
test for, because getting it wrong silently breaks every wrapped tool's help
output.

### Console build only

`--wrap`, `--adapt`, `--help`, `--version` and `--check` exist **only in the
console build**. The windowless `-w` build is a pure runtime payload: it is
installed into the wrapper folder and never handles its own flags.

This removes the need for `AttachConsole(ATTACH_PARENT_PROCESS)` in the `-w`
build, which would otherwise be necessary for its own output to be visible at
all.

Consequence to resolve: the console build installs by copying *itself*, so it
cannot produce a windowless wrapper unaided. Resolution: if an
`exe-env-wrapper-w*.exe` sits beside the console binary, `--wrap` copies that
one in preference and says which it used — background agents being the main
use case. `--console` forces a self-copy. If no sibling `-w` build is present,
`--wrap` copies itself and notes that the result is a console wrapper.

### `--wrap <name>` — the PATH shadowing mode

For callers that invoke by bare name and let `PATH` resolve it.

The argument takes either form:

| Argument | Meaning | Config written |
|---|---|---|
| `--wrap yt-dlp` | resolve on `PATH`, excluding the wrapper folder | none — resolution stays dynamic |
| `--wrap C:\tools\yt-dlp.exe` | use exactly that file | `target:` pinned to that path |

A bare name leaves resolution dynamic at runtime, which is the point: the real
binary can be upgraded, or move directory, without invalidating anything. A
full path is for pinning one specific file, and has to be recorded in a config
because it cannot be rediscovered.

Steps:

1. Resolve the argument by the table above. Refuse anything that is not an
   `.exe`.
2. Create `%USERPROFILE%\.local\wrapper` (`~/.local/wrapper`) if absent.
3. Copy the wrapper binary in as `<name>.exe`, preserving the `-w`/console
   choice described above.
4. Write a config **only if needed**: never for a bare name, always for a full
   path. See "No config file means defaults" below.
5. Verify the wrapper folder precedes the real binary's directory on `PATH`,
   and report if it does not, naming both positions.

The original binary is never touched, never renamed, never overwritten.

### No config file means defaults

A wrapper with no config file beside it applies a **built-in default
environment** — the system proxy block — and resolves its target by
next-on-PATH:

```yaml
env:
  HTTP_PROXY: ${system.http_proxy}
  HTTPS_PROXY: ${system.https_proxy}
  NO_PROXY: ${system.no_proxy}
```

So the common case needs no config file at all: `--wrap yt-dlp` copies one
binary in and is done. `--wrap yt-dlp --config` additionally writes
`yt-dlp.yaml` spelling those defaults out, as a starting point for editing.

This replaces today's behaviour, where a missing config is a hard error. The
error is kept for the case where there is also no next-on-PATH candidate, so a
wrapper that can do nothing useful still says so rather than failing
mysteriously.

### `--adapt <file>` — the in-place mode

For the case `--wrap` cannot serve: a binary whose **absolute path is
hard-coded into a script**, so `PATH` is never consulted.

The argument must be a full path, or a bare filename that exists in the
wrapper's own directory. `--adapt` never consults `PATH` — a name resolved
from `PATH` would be the wrong file to adapt in place, since the whole reason
to adapt is that the caller is not using `PATH`. `.exe` only, as above.

Steps:

1. Refuse if the file is itself an exe-env-wrapper build (marker scan), with a
   message saying it is already adapted. Without this check the wrapper would
   be renamed aside and the new config pointed at a wrapper — indirection that
   `ResolveTarget`'s existing self-target check does not catch, because the two
   are different files.
2. Hash the file's contents, rename it to `<name>-<hash>.exe`.
3. Copy the wrapper in under the original name.
4. Write `<name>.yaml` with an explicit `target: <name>-<hash>.exe` plus the
   same live system proxy block.

No PATH ordering check applies here: no shadow directory is involved.

**Hash choice.** `sha256` truncated to 8 hex characters —
`yt-dlp-3f9a1c2b.exe`. The hash is a content tag, not a security boundary, so
truncation is fine; `sha256` avoids the scanner and FIPS-policy friction that
`md5` attracts, at no cost. Flagged for review, as `md5` was the original
request.

**Idempotency.** If the hashed file already exists with matching content, the
rename is skipped rather than repeated. If `<name>.yaml` already exists it is
left alone and reported, never overwritten — losing a hand-edited config to a
re-run would be worse than refusing. `--force` overrides both.

### PATH ordering verification

Checked at install time by `--setup` and `--wrap`, and by `--check` on
demand. **Not checked at runtime.**

A runtime check cannot fire in the case it would be guarding against: if the
wrapper folder is not ahead of the real binary, the wrapper is never the one
invoked, so there is no run in which to notice. The check only has meaning
before or outside a wrapped invocation.

### `--setup` — create the folder and claim PATH

`--setup` creates `~/.local/wrapper` and prepends it to `PATH`. It is a
separate, explicit command because editing `PATH` is a persistent change to
the user's environment, and must never be a silent side effect of `--wrap`.
Without `--setup`, `--wrap` reports what needs adding and stops short of doing
it.

**Which PATH, and the constraint behind it.** Windows composes the effective
`PATH` as every machine entry first, then every user entry. A user entry can
therefore never precede a machine entry, whatever the order within each list.

| | Writes to | Elevation | Can shadow |
|---|---|---|---|
| `--setup` | user `PATH` (`HKCU\Environment`) | no | anything in user `PATH` |
| `--setup --machine` | machine `PATH` (`HKLM\...\Environment`) | yes | anything at all |

The default is the user `PATH`, prepended so the wrapper folder leads the user
section. On the development machine that places it at effective position 13,
immediately ahead of `~/.local/bin` — which is where the tools this exists for
actually live, `uv`-installed tools and `claude.exe` among them.

`--machine` exists for the remainder: tools in machine `PATH` entries, such as
`git`, `node`, `gh`, `go` and `dotnet` on this machine, cannot be shadowed
from the user `PATH` at all. Those are also reachable with `--adapt`, which
needs no `PATH` change.

**`--setup` reports what it achieved**: the position claimed, and which `PATH`
entries sit ahead of the wrapper folder and so cannot be shadowed without
`--machine`. A user running `--setup` and then finding `git` unwrapped should
learn why from the setup output, not from debugging later.

`--setup` is idempotent: already present and correctly positioned is reported
and left alone; present but too late in the list is moved to the front.

### `--check`

Audits the wrapper folder: for each wrapper in it, reports the resolved
target, whether the ordering is correct, and whether the config parses. The
diagnostic for "this stopped working and I do not know why".

### `--help`

Covers the YAML keys (`target`, `env`, `log`), `${VAR}` and `${system.*}`
expansion, both install modes and when to use each, the two Windows builds,
and the wrapper folder's `PATH` requirement.

A bare invocation under the natural name with no arguments prints help, rather
than today's error about a missing `exe-env-wrapper.yaml`, which tells a
first-time user nothing.

### Exit codes

| Code | Meaning |
|---|---|
| target's own | normal wrapped execution |
| 125 | the wrapper could not start the target (unchanged) |
| 0 | an install command succeeded, or `--check` found no problems |
| 1 | an install command failed, or `--check` found at least one problem |

Install failures are distinct from 125, which specifically means "could not
start the target"; an install command never starts one.

## Testing strategy

Test-driven throughout. The design deliberately concentrates the logic in pure
functions so the bulk is testable on any platform.

**Pure, table-driven, no filesystem or registry:**

- WinInet translation: bare vs per-scheme `ProxyServer`, scheme already
  present, `<local>` in first/middle/last position, `ProxyEnable = 0` with a
  non-empty override, empty and missing values, `socks=` present alongside
  `http=`, duplicate entries after `<local>` expansion.
- Next-on-PATH resolution, against an injected `PATH` and a fake filesystem:
  own directory listed twice under different spellings, own directory listed
  last, a wrapper as the next candidate, no candidate at all.
- `${system.*}` expansion: each of the three names, composition with literal
  text, unknown `system.` name errors, interaction with `${VAR}`, `$${...}`
  staying literal.

**Behavioural, with real files in a temp directory:**

- `--help` under the natural name prints help; `--help` under an adapted name
  reaches the target untouched. Both directions, as this is the regression that
  matters most.
- `--adapt` renames, copies, and writes the config; refuses a wrapper; is
  idempotent on re-run; does not overwrite an existing config without
  `--force`.
- `--wrap <name>` resolves via `PATH` and installs without a config file;
  `--wrap <full path>` pins an explicit `target`; both refuse a non-`.exe`.
- `--wrap` and `--setup` report a bad `PATH` order at install time.
- `--setup` against an injected `PATH` pair: creates the folder, prepends to
  the user list, is idempotent when already leading, moves the entry forward
  when present but too late, and names the machine entries it cannot shadow.
  The registry write itself is behind a seam so the ordering logic is tested
  without touching the real environment.
- Unknown top-level config keys are rejected by name.

**Windows-only plumbing:** one test that performs the real registry read and
asserts it does not error. It must tolerate a machine with no proxy configured
— this development machine has `ProxyEnable = 0` and an empty `ProxyOverride`,
and `netsh winhttp` reports direct access, so the "nothing configured" path is
the one that gets exercised locally.

## Documentation changes

- Config reference: optional `target`, the `${system.*}` names and the
  translation rules.
- A new installation section leading with `--wrap`, with `--adapt` presented as
  the fallback for hard-coded paths.
- Remove the `uv tool upgrade` caveat from the `--wrap` path, where it no
  longer applies, and keep it against `--adapt`, where it does.
- The wrapper folder, its `PATH` requirement, and `--setup`.

## Implementation order

Each step ends green with the suite passing.

1. `Expand` lookup signature gains an `error`. Pure refactor, no behaviour
   change.
2. WinInet translation as a pure function, with its table-driven tests.
3. The registry read and the non-Windows fallback, wired into expansion.
4. Known-fields-only config decoding.
5. Optional `target` plus next-on-PATH resolution (`.exe` only), including
   the marker, and the built-in default environment applied when no config
   file is present.
6. The name gate, `--help` and `--version`, with the pass-through regression
   test.
7. `--setup`, `--wrap`, and the install-time PATH ordering check.
8. `--adapt`.
9. `--check`, and `--setup --machine`.
10. README.

Steps 1-4 deliver the proxy fix on its own and are independently useful; the
`--wrap` work in 5-7 is what retires the upgrade problem.
