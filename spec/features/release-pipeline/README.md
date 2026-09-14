---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Release and verification pipeline

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/release-pipeline?op=explore) | [Edit](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/release-pipeline?op=edit) | [Ask question](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/release-pipeline?op=ask) | [Request change](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/release-pipeline?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Purpose

A coverage tool is only as trustworthy as the binary the user actually runs, and
a self-updating binary is only as safe as the pipeline that produced the bytes it
downloads. This feature fixes what must be true before a tag becomes a release:
a tidy module, tests on every supported OS, a real coverage floor, a
cross-compiled artefact matrix with checksums, a CI gate on the tagged commit,
and a self-update path whose failure modes leave a working binary behind.

### REQ: tidy-module

`go.mod` and `go.sum` MUST be tidy. CI MUST fail on `go mod tidy -diff` drift
rather than silently rewriting the module file, because a release must be built
from exactly the tagged tree: a workflow that rewrites dependencies during
release produces a binary nobody reviewed and makes the tag an unreliable
description of its own contents.

### REQ: cross-platform-ci

CI MUST run the unit tests on Linux, macOS, and Windows, and MUST run the
SpecScore spec-tree lint as a gate. `cover100` walks foreign directory trees,
spawns `go test` and Node runners, and manages processes; those paths differ per
OS, and the spec tree is the contract this repository ships alongside the binary,
so neither may be left unverified.

### REQ: coverage-floor

CI MUST enforce a minimum test-coverage floor of 100% of statements over the
project's own packages, measured with `go tool cover -func`.

The floor MUST be a real number that the repository actually passes at the time
it is set. A floor that is aspirational, or that is never checked, is worse than
no floor: it trains reviewers to ignore the gate.

A 100% floor is only honest when every branch is reachable on some supported
platform, so platform-specific behaviour MUST live in build-tagged files
(`browser_darwin.go`, `runner_suffix_windows.go`) rather than in a
`switch runtime.GOOS` or an `if runtime.GOOS == ...` inside shared code: GOOS is
a compile-time constant, so on any single platform the other arms are dead code
that the coverage tool still counts against the denominator. A guard that cannot
fail — one that only restates a standard-library guarantee — MUST be deleted
rather than left permanently uncovered.

### REQ: goreleaser-artifacts

GoReleaser MUST build `CGO_ENABLED=0` binaries for darwin, linux, and windows on
both amd64 and arm64. Static binaries are what make a downloaded release
runnable on a machine without a matching C toolchain.

The build MUST stamp version, commit, and date through
`github.com/strongo/buildinfo`'s `-X` variables, so that a released binary can
report its own identity.

The release MUST publish plain GitHub release archives plus a checksums file
named `cover100_<version>_checksums.txt`, with archives named
`cover100_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows). Those names are not
cosmetic: they are `github.com/strongo/cli-helpers/selfupdate`'s own defaults,
and `self-update` resolves an asset by that pattern. A checksums file under any
other name would be invisible to it, so every update would fail closed.

### REQ: release-gate

A release MUST be refused unless the CI workflow for the tagged commit
succeeded. Building release artefacts from a commit whose tests, coverage floor,
or spec lint failed would ship a binary that the repository's own gates rejected.

### REQ: safe-self-update

`self-update` MUST be safe by construction. A package-manager-owned install MUST
never be overwritten. Checksum verification MUST happen before any bytes are
extracted, so a corrupted or tampered archive cannot touch the filesystem. The
binary swap MUST be atomic, so that an interrupted or failed update leaves a
working binary rather than a truncated one.

## Acceptance criteria

### AC: tidy-drift-fails-ci

**Given** a branch whose `go.mod` or `go.sum` differs from `go mod tidy` output

**When** CI runs on that branch

**Then** the check reports the `go mod tidy -diff` drift and fails the workflow

**And** the workflow does not rewrite the module files in place.

**Requirements:** release-pipeline#req:tidy-module

### AC: cross-platform-test-matrix

**Given** a commit on any branch

**When** CI runs

**Then** unit tests execute on Linux, macOS, and Windows, and the SpecScore spec
lint runs as a required gate

**And** a failure on any single OS or in the spec lint fails the workflow as a
whole.

**Requirements:** release-pipeline#req:cross-platform-ci

### AC: coverage-floor-enforced

**Given** the configured minimum coverage floor of 100% and the project's own
packages

**When** CI measures coverage with `go tool cover -func`

**Then** the measured value is compared against the floor and the workflow fails
when coverage falls below it

**And** the floor is a concrete number that the repository passes at HEAD, with
no function reported below 100%.

**Requirements:** release-pipeline#req:coverage-floor

### AC: release-artifact-matrix

**Given** a tagged release built by GoReleaser

**When** the published artefacts are inspected

**Then** archives exist for darwin, linux, and windows on amd64 and arm64, every
binary was built with `CGO_ENABLED=0`, version, commit, and date are stamped via
`github.com/strongo/buildinfo`'s `-X` variables, and
`cover100_<version>_checksums.txt` is published alongside them

**And** every archive has exactly one matching sha256 entry in that checksums
file, under the name `self-update` resolves.

**Requirements:** release-pipeline#req:goreleaser-artifacts

### AC: release-blocked-without-green-ci

**Given** a tag whose commit has a failing or unfinished CI workflow, and a tag
whose commit has a successful one

**When** a release is attempted for each

**Then** the release is refused for the failing or unfinished commit and
proceeds only for the commit with a successful CI workflow for that tag.

**Requirements:** release-pipeline#req:release-gate

### AC: self-update-safety

**Given** a package-manager-owned installation, an archive whose sha256 does not
match the release checksums, an archive with no matching entry, and an update
interrupted mid-swap

**When** `self-update` runs in each case

**Then** the package-manager-owned install is never overwritten, the mismatched
archive is rejected, the archive with no checksum entry is refused, and after an
interrupted swap a working binary still runs

**And** checksum verification completes before any bytes are extracted.

**Requirements:** release-pipeline#req:safe-self-update

## Open Questions

- Should the coverage floor be raised automatically when measured coverage
  improves, or left as a manually reviewed constant?
- Should CI also run the spec lint with `--severity warning`, matching the local
  authoring command, or keep the default severity?
- Should release artefacts include a `latest` pointer for `self-update` to
  resolve, or is resolving the newest non-prerelease tag sufficient?

---
*This document follows the https://specscore.md/feature-specification*
