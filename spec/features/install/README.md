---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Install

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/install?op=explore) | [Edit](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/install?op=edit) | [Ask question](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/install?op=ask) | [Request change](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/install?op=request-change) |
**Status:** Implementing
**Source Ideas:** —

## Summary

`cover100 install` lists and installs the fleet CLIs relevant to cover100,
and `cover100 upgrade` reports and upgrades every installed catalog CLI plus
cover100 itself, both built entirely on the shared
[CLI Install Command Library](https://github.com/strongo/cli-helpers/blob/main/spec/features/cli-install/README.md).
`cover100 self-update` is `cover100 upgrade cover100`: both reach the
identical library call, because cover100 is always upgraded last and
classified from its own self-update Config, never a `PATH` probe of its own
binary.

## Problem

cover100's catalog entry declares it relevant to `codegrapher` and `wb` (the
[CLI Install Command Library](https://github.com/strongo/cli-helpers/blob/main/spec/features/cli-install/README.md)'s
relevance matrix), and each of those CLIs' own `install` command lists
`cover100` in turn. Without `cover100 install` itself, a user of one of those
CLIs could discover and install `cover100`, but a user who started from
`cover100` had no equivalent way to discover CodeGrapher or `wb`, or to
install either consistently with how they installed `cover100`.

## Behavior

### Built on the shared library

`cover100 install` is built entirely on the fleet-wide CLI Install Command
Library (`github.com/strongo/cli-helpers/cliinstall`, Implementing),
configured from cover100's own compiled-in catalog entry — the same entry
`self-update` builds its `selfupdate.Config` from
(cli-install#req:host-identity-from-catalog,
cli-install#req:catalog-identity-single-source). Listing, details, the
destination decision, the confirmation gate, checksum-verified download and
`--dry-run`/`--format json` are all specified once in that library, not
restated here; this Feature specifies only cover100's own host id, its exit-
code mapping, and how the verb sits beside the root command's own optional
`[path]` argument.

### Command surface

#### REQ: command-name

The CLI MUST expose the command as `cover100 install`, built from
`github.com/strongo/cli-helpers/cliinstall/cobracmd`. The command inherits
the library's full flag surface — `--all`, `--yes`/`-y`, `--dry-run`,
`--dir`, and `--format text|json` — none of which is re-specified here.
`install` MUST NOT gain an `update` alias
(cli-install#req:update-alias-policy): cover100 ships no `update` alias
anywhere today.

#### REQ: upgrade-command

The CLI MUST expose `cover100 upgrade [name...]`, built from
`github.com/strongo/cli-helpers/cliinstall/cobracmd`'s `cobracmd.NewUpgrade`.
The command inherits the library's full upgrade flag surface — `--all`,
`--check`, `--yes`/`-y`, `--dry-run`, and `--format text|json` — none of
which is re-specified here, and MUST carry no `update` alias
(cli-install#req:update-alias-policy). `upgrade`'s `HostConfig` MUST be the
exact SAME Config (resolved through the `selfUpdateConfigFunc` seam)
`self-update` itself builds, so `cover100 self-update` and `cover100 upgrade
cover100` reach the identical library call
(cli-install#req:self-update-equals-upgrade-self,
cli-install#req:host-target-is-running-binary). cover100's self-update
configures no after-update hook, so `HostAfterUpdate` is left nil. `cover100
upgrade` (no further arguments recognizable as a path) MUST resolve to the
`upgrade` subcommand's bare report, while `cover100 ./upgrade` MUST still be
treated as the root command's `path` argument, following the SAME Cobra
command-resolution rule [REQ: install-verb-vs-path-argument](#req-install-verb-vs-path-argument)
already states for `install`.

#### REQ: install-verb-vs-path-argument

cover100's root command already accepts an optional `[path]` positional
argument ([cli-command-surface#req:invocation-and-flags](../cli-command-surface/README.md#req-invocation-and-flags)).
Registering `install` as a subcommand MUST NOT change that: `cover100
install` (no further arguments recognizable as a path) MUST resolve to the
`install` subcommand's bare listing, while `cover100 ./install` (a name that
is not a registered subcommand once it carries a path prefix) MUST still be
treated as the root command's `path` argument, scanning a directory literally
named `install`. This follows from Cobra's own command resolution — the
first token is matched against registered subcommand names before falling
back to the current command's own positional arguments — and needs no code
beyond registering the command.

### Host identity

#### REQ: host-id

`cover100 install` MUST identify the host to the library as catalog id
`"cover100"` only (`cobracmd.CommandOptions.HostID`), never a hand-written
duplicate of the catalog entry (cli-install#req:host-identity-from-catalog).
`cliinstall.ByID("cover100")` is the same entry `self-update` resolves
(`cliinstall/catalog_cover100.go` in `strongo/cli-helpers`), so `install`'s
relevant-targets listing, and every other fleet CLI's `install cover100`,
resolve cover100's own release identically. `cobracmd.New` panics if
`"cover100"` is absent from the compiled catalog — a programming error
`TestNewInstallCmd_Registration` and `TestNewInstallCmd_ResolvesAgainstTheCatalog`
catch, never a runtime state a user sees.

### Exit codes

#### REQ: exit-codes

Unlike a host with a single generic exit code, cover100 already distinguishes
a usage mistake from an unexpected runtime failure for every other command it
has (see [cli-command-surface#req:exit-codes-and-output](../cli-command-surface/README.md#req-exit-codes-and-output)).
`install`'s error mapper follows the same two exit codes: `selfupdate.KindUnknownTarget`
— mapped through an EXPLICIT branch, never a self-update default branch —
maps to cover100's usage exit code `2`; every other failure — the other two
cli-install-only kinds, any self-update-shared kind (download, checksum,
permission, non-interactive refusal, a managed-command failure, ...), and an
already-`*cobracmd.UsageError` (an invalid `--format`, or `--all` combined
with names) — maps to cover100's general failure exit code `10`
(cli-install#req:host-owned-exit-codes). `install nosuchcli` MUST exit `2`
and name the unknown target. `upgrade` MUST use the exact SAME `installErrors`
mapper (cli-install#req:host-owned-exit-codes: "The upgrade command MUST use
the same error mapper"); it declares no upgrades-available method, so
`upgrade --check` never signals a dedicated exit code for an available
update — matching `self-update`'s own `UpdateAvailable`, which always
returns nil (informational only). `upgrade nosuchcli` MUST exit `2` and name
the unknown target, matching `install nosuchcli` exactly.

| Exit code | Meaning |
|---|---|
| `0` | Success: every named target installed/upgraded, already installed/current, redirected, or dry run — including `upgrade --check` regardless of verdict |
| `2` | Usage: an unknown install/upgrade target |
| `10` | Any other failure: no usable install directory, a destination that already exists, any self-update-shared failure kind, or an invalid `--format`/`--all` usage |

## Implementation

Source files implementing this feature:

- [`internal/cli/install.go`](../../../internal/cli/install.go) — the
  `installErrors` exit-code mapper and the `cobracmd.New` wiring against
  `HostID: "cover100"`.
- [`internal/cli/upgrade.go`](../../../internal/cli/upgrade.go) — the
  `cobracmd.NewUpgrade` wiring, reusing `installErrors` and resolving
  `HostConfig` through the same `selfUpdateConfigFunc` seam `self-update`
  uses.
- [`internal/cli/root.go`](../../../internal/cli/root.go) — registers
  `newInstallCmd()` and `newUpgradeCmd()` on the root command.

The shared behavior lives upstream, not in this repository:
`github.com/strongo/cli-helpers` `cliinstall/`, `cliinstall/cliui/`,
`cliinstall/cobracmd/` (the library and its Cobra adapter), and
`cliinstall/catalog_cover100.go` (cover100's own catalog entry, shared with
`self-update`).

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI command surface and static server](../cli-command-surface/README.md) | `install` and `upgrade` are registered on the same root command as the `[path]`-accepting collect verb and the `self-update` verb; [REQ: install-verb-vs-path-argument](#req-install-verb-vs-path-argument) states how they coexist. |
| self-update (`internal/cli/self_update.go`) | `install` and `upgrade` build from the same `cliinstall.ByID("cover100")` catalog entry and the same `selfUpdateConfigFunc`-resolved `selfupdate.Config` self-update itself builds, so their view of cover100 (shown by other CLIs) and `self-update`'s own release identity never disagree, and `self-update`/`upgrade cover100` reach the identical library call (cli-install#req:self-update-equals-upgrade-self). |

## Acceptance Criteria

### AC: registration-and-host-id

**Requirements:** install#req:command-name, install#req:host-id

**Given** the compiled `cliinstall` catalog

**When** `newInstallCmd()` builds the command

**Then** it registers `--all`, `--yes`/`-y`, `--dry-run`, `--dir` and
`--format`, resolves against catalog id `"cover100"`, and carries no
`update` alias.

### AC: unknown-target-exit-code

**Requirements:** install#req:exit-codes

**Given** the real command built exactly as `root.go` wires it

**When** the user runs `cover100 install nosuchcli`

**Then** the command fails before any confirmation, network request or
write, names `nosuchcli` in its error, and exits `2`.

### AC: install-verb-resolves-over-path

**Requirements:** install#req:install-verb-vs-path-argument

**Given** the root command with `install` registered

**When** `cover100 install` and `cover100 ./install` are each resolved
through the root command's own command lookup

**Then** the first resolves to the `install` subcommand and the second
resolves to the root command itself with `./install` as its `path` argument.

### AC: upgrade-registration-and-self-update-equivalence

**Requirements:** install#req:upgrade-command, cli-install#req:self-update-equals-upgrade-self

**Given** the compiled `cliinstall` catalog
**When** `newUpgradeCmd()` builds the command, and separately `cover100 self-update --check` and `cover100 upgrade cover100 --check` run against the same release state
**Then** the command registers `--all`, `--check`, `--yes`/`-y`, `--dry-run` and `--format`, carries no `update` alias, and both commands report the same current/latest verdict.

### AC: upgrade-unknown-target-exit-code

**Requirements:** install#req:exit-codes

**Given** the real command built exactly as `root.go` wires it
**When** the user runs `cover100 upgrade nosuchcli`
**Then** the command fails before any confirmation, network request or write, names `nosuchcli` in its error, and exits `2`, matching `install nosuchcli` exactly.

### AC: upgrade-verb-resolves-over-path

**Requirements:** install#req:upgrade-command

**Given** the root command with `upgrade` registered
**When** `cover100 upgrade` and `cover100 ./upgrade` are each resolved through the root command's own command lookup
**Then** the first resolves to the `upgrade` subcommand and the second resolves to the root command itself with `./upgrade` as its `path` argument.

The remaining behavior — the relevance matrix, listing and status probing,
destination policy, checksum-verified direct installs, the confirmation
gate, `--dry-run`, and `--format json` — is specified and tested once in the
[CLI Install Command Library](https://github.com/strongo/cli-helpers/blob/main/spec/features/cli-install/README.md)'s
own Acceptance Criteria, which this command inherits by construction rather
than re-proving.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
