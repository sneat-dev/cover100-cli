---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: CLI command surface and static server

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/cli-command-surface?op=explore) | [Edit](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/cli-command-surface?op=edit) | [Ask question](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/cli-command-surface?op=ask) | [Request change](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/cli-command-surface?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Purpose

`cover100` is a single binary that collects, writes, serves, and opens a report.
This feature fixes what a user types, what the process does with the filesystem
and the network, what it prints on each stream, and what exit code it returns —
the contract that scripts and CI depend on. It also fixes which shared
fleet libraries provide that surface, so the CLI behaves like its siblings
rather than growing a hand-rolled equivalent.

### REQ: invocation-and-flags

The command MUST be invoked as `cover100 [path] [flags]`, with `path` defaulting
to the current directory. Path defaulting means the common case is a bare
`cover100` in the repository being measured.

The command MUST accept these flags with these exact defaults:

| Flag | Default | Meaning |
|---|---|---|
| `--out` | `.cover100/coverage.json` | where the report JSON is written |
| `--open` / `--no-open` | open | whether to open the report in a browser |
| `--lang` | `all` | `go`, `ts`, `js`, or `all`; `ts` and `js` restrict collection by file extension, `go` restricts to Go projects |
| `--metric` | `lines` | initial metric |
| `--mode` | `percent` | initial sizing mode, `percent` or `count` |
| `--port` | `5173` | preferred static server port |
| `--keep` | `false` | retain intermediate artefacts |
| `--no-serve` | `false` | write the report and exit without serving |
| `--file` | `false` | open the self-contained document over `file://` |
| `--timeout` | `15m` | timeout applied to each project's coverage command |
| `--format` | `text` | `text` or `json` |
| `--verbose` | `false` | verbose diagnostics |

`--metric` and `--mode` MUST be passed through to the page as query parameters.
Passing them is what keeps the CLI's default and the UI's initial state in
agreement; without it the page would open on its own default while the CLI
claimed otherwise.

### REQ: static-server

Page assets MUST be embedded in the binary with `go:embed` and served from an
HTTP server bound to loopback only. A local coverage report must not be exposed
to the network, and embedding keeps the binary self-contained and independent of
the working directory.

The report MUST be served at `/coverage.json`. If the requested port is already
in use, the server MUST try subsequent ports and MUST report the port it actually
bound, so that a busy 5173 is an inconvenience rather than a failure.

The browser MUST be opened at
`http://127.0.0.1:<port>/?data=/coverage.json&metric=…&mode=…` using the port
that was actually bound.

### REQ: run-modes

`--no-serve` MUST write the report and exit without starting a server.
`--file` MUST write and open the self-contained document over `file://` and MUST
start no server. `--no-open` MUST still serve the report and print its URL, so
that a user who wants to open the page manually, or a script that wants the URL,
is not forced into launching a browser.

### REQ: signal-handling

On Ctrl+C (SIGINT) or SIGTERM, `cover100` MUST terminate any running coverage
command, shut the server down, and exit cleanly with code 130. Leaving orphaned
`go test` or test-runner processes alive would corrupt the next run and would
keep the terminal busy; a conventional 130 exit code makes interruption
scriptable.

### REQ: artifact-lifecycle

Intermediate cover profiles and coverage directories MUST live under the output
directory and MUST be deleted on exit unless `--keep` is set. Scattering
temporary artefacts through the scanned tree would make `cover100` a source of
the very noise it is meant to measure.

The generated report JSON and the standalone HTML MUST NEVER be deleted by that
cleanup, including when `--out` points inside the same directory. The whole point
of the run is to produce those two files; a cleanup that can remove them is a
data-loss bug.

### REQ: exit-codes-and-output

The command MUST exit `0` on success, including a run that completed with
collection warnings; `2` for invalid arguments; `3` when the scan path does not
exist or no supported project was found; and `10` for an unexpected runtime
failure. Partial collection failures MUST be warnings and MUST NOT produce a
non-zero exit, and every such warning MUST appear both on stderr and in the
document's `warnings` array, so that an interactive user and a machine consumer
see the same facts.

Progress and the summary MUST be written to stdout; warnings, errors, and
diagnostics MUST be written to stderr. `--format json` MUST emit a single
machine-readable run summary on stdout and nothing else, so that the command is
scriptable with a plain JSON parse.

### REQ: fleet-stack-reuse

The command surface MUST be built on the shared fleet stack rather than
hand-rolled equivalents: `spf13/cobra` fronted by `charm.land/fang/v2` via
`github.com/strongo/buildinfo/fangcmd.Wire`; version identity from
`github.com/strongo/buildinfo`; diagnostics through `github.com/strongo/logus`;
and a `self-update` verb built from
`github.com/strongo/cli-helpers/selfupdate/cobracmd`. Reuse is what keeps help
formatting, version reporting, logging, and self-update behaviour identical
across the fleet's CLIs and fixes them in one place.

## Acceptance criteria

### AC: default-invocation-and-flag-defaults

**Given** a repository and an installed `cover100`

**When** it is invoked with no arguments and inspected with `--help`

**Then** `path` defaults to the current directory, every flag in the table above
is present with its stated default and meaning, and `--lang ts`/`--lang js`
restrict collection by file extension while `--lang go` restricts to Go projects

**And** the report page is opened with `metric=` and `mode=` query parameters
matching the effective `--metric` and `--mode` values.

**Requirements:** cli-command-surface#req:invocation-and-flags

### AC: embedded-server-and-port-fallback

**Given** a build of `cover100` and a machine where port 5173 is already bound

**When** the command serves a report

**Then** the page assets come from the binary's `go:embed` filesystem, the
listener is bound to loopback only, `/coverage.json` serves the report, the
server binds the next free port, and the actually bound port is reported and used
in the browser URL

**And** the opened URL is
`http://127.0.0.1:<port>/?data=/coverage.json&metric=…&mode=…`.

**Requirements:** cli-command-surface#req:static-server

### AC: serve-less-and-file-modes

**Given** a successful collection

**When** the command runs with `--no-serve`, then with `--file`, then with
`--no-open`

**Then** `--no-serve` writes the report and exits with no listener started,
`--file` writes and opens a self-contained document over `file://` with no
listener started, and `--no-open` still serves the report and prints its URL
without launching a browser.

**Requirements:** cli-command-surface#req:run-modes

### AC: interrupt-shutdown

**Given** a `cover100` run in progress with a coverage command executing and the
static server listening

**When** the process receives SIGINT or SIGTERM

**Then** the running coverage command is terminated, the server is shut down and
the process exits with code 130

**And** no child coverage process and no listener remain after exit.

**Requirements:** cli-command-surface#req:signal-handling

### AC: artifact-cleanup-safety

**Given** a run whose intermediate profiles and coverage directories live under
the output directory, and a variant in which `--out` points inside that same
directory

**When** the run exits normally, and separately when it exits with a failure

**Then** the intermediate artefacts are deleted unless `--keep` was given, and
with `--keep` they are retained

**And** the generated report JSON and any standalone HTML are never deleted by
cleanup, including in the `--out`-inside-directory variant.

**Requirements:** cli-command-surface#req:artifact-lifecycle

### AC: exit-codes-and-warning-routing

**Given** four runs: a clean run, a run with a malformed flag, a run against a
non-existent path or a tree with no supported project, and a run that hits an
unexpected runtime failure, plus a run where one project fails to collect

**When** each run terminates

**Then** the exit codes are `0`, `2`, `3`, and `10` respectively, the partial
collection failure exits `0`, and its warning appears both on stderr and in the
document's `warnings` array.

**Requirements:** cli-command-surface#req:exit-codes-and-output

### AC: machine-readable-output-and-fleet-wiring

**Given** a `cover100` invocation with `--format json`

**When** it completes

**Then** stdout contains exactly one machine-readable run summary and nothing
else, while progress and the summary of a `text` run go to stdout and warnings,
errors, and diagnostics go to stderr

**And** the command is built on `spf13/cobra` fronted by `charm.land/fang/v2`
through `github.com/strongo/buildinfo/fangcmd.Wire`, reports version identity
from `github.com/strongo/buildinfo`, logs through `github.com/strongo/logus`,
and exposes a `self-update` verb built from
`github.com/strongo/cli-helpers/selfupdate/cobracmd`.

**Requirements:** cli-command-surface#req:exit-codes-and-output, cli-command-surface#req:fleet-stack-reuse

## Open Questions

- Should a project that exceeds `--timeout` be reported as a warning or as a
  distinct exit code, and should the same bound also apply to the whole run?
- Should `--port 0` be a documented way to ask the OS for any free port for
  scripted use?
- Should `--lang` also accept explicit extension lists (for example `--lang
  tsx,jsx`) alongside the four named values?

---
*This document follows the https://specscore.md/feature-specification*
