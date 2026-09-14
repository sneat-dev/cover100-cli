---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Coverage collection and normalization

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/coverage-collection?op=explore) | [Edit](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/coverage-collection?op=edit) | [Ask question](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/coverage-collection?op=ask) | [Request change](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/coverage-collection?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Purpose

`cover100` answers one question at the resolution where it is actionable: which
parts of a polyglot repository are untested? Collection is the feature that turns
two unrelated coverage toolchains into one normalized document, so that neither
the renderer nor a downstream script needs to know whether a percentage came
from Go's cover profile or from Istanbul. Normalization lives here and only
here; every consumer reads the same tree.

### REQ: project-detection

A scan of the target directory tree MUST find every Go module (a directory
containing `go.mod`) and every Node package (a directory containing
`package.json`). Relative to the scan root, the scan MUST NOT descend into a
directory whose base name begins with `.`, nor into `node_modules` or `vendor`:
those trees are dependency payloads, and walking them would report other
people's code as the user's coverage.

A directory that holds both `go.mod` and `package.json` MUST become ONE
repository node, not two. The repository is the unit of relative pathing and of
concurrent collection; detecting it twice would double-count its metrics and
misattribute every path below it.

Independent repositories MUST be collected concurrently, and a failure in one
repository's collection MUST NOT block, cancel, or corrupt another
repository's result.

### REQ: go-profile-collection

For each discovered module, `cover100` MUST run
`go test -coverprofile=<workdir>/go-<n>.out ./...` with the working directory
set to the module directory, where `<n>` distinguishes modules within the run.

The cover profile MUST be parsed directly from the text format: an optional
`mode:` header line followed by records of the form
`file:startLine.startCol,endLine.endCol numStmts count`. No `gocov` or other
external tool may be invoked or required. The format is small and stable, an
offline CLI must not depend on a toolchain the user did not install, and a
direct parser is the only way to apply the specified line semantics.

A profile block MUST contribute every line in its inclusive
`startLine..endLine` range to the file's line set. A line's hit count MUST be
the maximum count over all blocks that cover it. `lines.total` MUST count the
distinct lines referenced by blocks and `lines.covered` MUST count the distinct
lines whose hit count is greater than zero. Files named `_test.go` MUST be
excluded from the tree.

A non-zero `go test` exit MUST NOT abort the run. Whatever profile the command
wrote before failing MUST still be parsed, and the failure MUST be recorded as a
warning; one broken test binary must not hide the coverage of every other
module.

### REQ: go-function-availability

Go cover profiles carry no function data. Every Go file node MUST therefore
report `functions: {covered: 0, total: 0}` and `functionsAvailable: false`.

The limitation MUST be surfaced rather than silently implied: it MUST appear in
the JSON document, in the CLI run summary, and in the UI tooltip, and it MUST be
documented in the README. A reader who sees `0 / 0` must be told that no
measurement was possible, not left to interpret it as full or zero function
coverage.

### REQ: node-runner-selection

For each `package.json` whose `test` script is non-empty, the collector MUST
select a runner from the package dependencies or, failing that, from the test
script itself, preferring `vitest` and then `jest`. It MUST prefer the package's
own `node_modules/.bin/<runner>` over `npx`, so that the version the project
pinned is the version that actually runs.

Jest MUST be invoked with
`--coverage --coverageReporters=json --coverageDirectory=<dir>` and Vitest with
`run --coverage.enabled=true --coverage.reporter=json --coverage.reportsDirectory=<dir>`,
with `<dir>` inside the work directory so that artefacts are attributable to the
package that produced them.

### REQ: node-coverage-parsing

The collector MUST parse Istanbul `coverage-final.json`. Lines MUST be derived
from `statementMap` plus `s`: a statement maps to its `start.line`, a line's hit
count is the maximum over statements starting on that line, and the `l` map MUST
be used verbatim when present because it is the instrumenter's own line
accounting. Functions MUST be derived from `fnMap` plus `f`, and branches from
`branchMap` plus `b` when present.

When `coverage-final.json` is absent, the collector MUST fall back to
`lcov.info` and MUST understand the `DA`, `FN`, `FNDA`, `BRDA`, and `SF`
records. A function parsed from lcov MUST carry
`lines: {covered: 0, total: 0}`, because lcov records the declaration line but
not the function body range needed to attribute lines.

A statement MUST belong to the innermost function whose `loc` contains the
statement's start position; a statement claimed by no function MUST belong to
the file only. A function node's `lines` MUST be the aggregate of the statements
it owns. Containment, not proximity, is the rule, so a nested inner function
does not leak its lines into its enclosing function.

A missing coverage artefact MUST be reported as a warning and MUST NOT fail the
run; a package that cannot produce coverage is a gap to disclose, not a reason
to discard the other projects' results.

### REQ: normalized-tree

The tree MUST be exactly `root → repository → package → file → function`. A
package is the directory holding a file, relative to its repository root; a
file's `path` is relative to its repository root; a repository node is keyed by
its directory relative to the scan root.

Node ids MUST be globally unique and stable and MUST be formed as `repo:<rel>`,
`pkg:<rel>`, `file:<rel>`, and `fn:<rel>#<name>@<line>`, where `<rel>` is
relative to the scan root. Ids are the handle used by the viewer's focus,
filter, and copy-path controls, so they must be reconstructible from the
document alone.

Line-level nodes MUST NOT be materialized. Per-line detail is aggregated into
file and function metrics instead, because a line node per covered statement
would multiply the document size by orders of magnitude and make the
self-contained HTML variant impractical.

### REQ: metric-aggregation

Every node MUST carry all five of `lines`, `functions`, `files`, `packages`, and
`repositories`, each as `{covered, total}`. Uniform shape is what lets the viewer
size and colour any node without special cases.

`lines`, `functions`, and `files` MUST roll up as element-wise sums of the
node's children. A `package` node's `packages` and `repositories` MUST be
`{covered: (lines.total > 0 && lines.covered > 0) ? 1 : 0, total: 1}`,
representing itself and its containing repository. A `repository` node's
`packages` MUST count its direct package children whose `lines.covered > 0`, and
its `repositories` MUST be `{covered: cov(lines), total: 1}` where `cov(lines)`
is 1 when `lines.covered > 0` and 0 otherwise. The root's `packages` MUST sum
its repositories' `packages`, and the root's `repositories` MUST count its
repository children whose `lines.covered > 0`.

A file node's `lines` and `functions` MUST be its own measured values, not the
sum of its function children, because those children are a subset of the file's
statements and summing them would understate the file.

Siblings MUST be sorted descending by `lines.total`, then by `functions.total`,
then by name, so that two runs over the same inputs produce byte-identical
documents.

## Acceptance criteria

### AC: mixed-repository-detection

**Given** a scan root containing a Go module, a Node package, a directory holding
both `go.mod` and `package.json`, and nested `node_modules`, `vendor`, and hidden
directories that each contain their own manifests

**When** `cover100` scans the tree

**Then** the Go module and the Node package each appear as one repository node,
the dual-manifest directory appears as exactly one repository node, and nothing
inside `node_modules`, `vendor`, or a hidden directory is detected

**And** every package and file path is resolved relative to the repository root
that actually contains it.

**Requirements:** coverage-collection#req:project-detection, coverage-collection#req:normalized-tree

### AC: go-profile-parsing

**Given** a module whose `go test -coverprofile=...` writes a profile with
overlapping blocks, a block whose count is zero, and `_test.go` entries

**When** `cover100` parses the profile

**Then** every line in each block's `startLine..endLine` range is counted once in
`lines.total`, a line covered by overlapping blocks takes the maximum count,
lines whose maximum count is zero are not counted as covered, and `_test.go`
entries are absent from the tree

**And** a non-zero `go test` exit still yields the parsed profile plus a warning
instead of an aborted run.

**Requirements:** coverage-collection#req:go-profile-collection

### AC: go-functions-reported-unavailable

**Given** a Go module with covered lines

**When** the coverage document is produced

**Then** every Go file node reports `functions: {covered: 0, total: 0}` and
`functionsAvailable: false`, the CLI summary states that Go function coverage is
unavailable, the UI tooltip exposes the same caveat, and the README documents
the limitation

**And** the zero function totals do not suppress or alter that file's `lines`
metrics.

**Requirements:** coverage-collection#req:go-function-availability

### AC: node-runner-selection-and-invocation

**Given** packages that depend on `vitest`, packages that depend on `jest`, a
package with neither dependency but a `test` script naming its runner, and a
package with a local `node_modules/.bin/jest`

**When** `cover100` collects Node coverage

**Then** vitest is preferred over jest, the runner is inferred from the test
script when the dependencies are silent, and the package's local binary is used
in preference to `npx`

**And** jest receives `--coverage --coverageReporters=json --coverageDirectory=<dir>`
and vitest receives
`run --coverage.enabled=true --coverage.reporter=json --coverage.reportsDirectory=<dir>`.

**Requirements:** coverage-collection#req:node-runner-selection

### AC: istanbul-and-lcov-normalization

**Given** one package that emits `coverage-final.json` and another that emits only
`lcov.info`

**When** `cover100` normalizes both

**Then** lines come from `statementMap` plus `s` with the `l` map used verbatim
when present, functions come from `fnMap` plus `f`, branches come from
`branchMap` plus `b` when present, and the `DA`, `FN`, `FNDA`, `BRDA`, and `SF`
records drive the lcov fallback

**And** functions parsed from lcov carry `lines: {covered: 0, total: 0}`, while a
package that emits neither artefact produces a warning and does not fail the run.

**Requirements:** coverage-collection#req:node-coverage-parsing

### AC: function-line-attribution

**Given** an Istanbul file whose statements start inside a top-level function,
inside a nested function, and outside every function

**When** function nodes are built

**Then** each statement is assigned to the innermost function whose `loc`
contains its start position, statements claimed by no function belong to the
file alone, and each function's `lines` is the aggregate of the statements it
owns

**And** the file's own `lines` remain its measured value rather than the sum of
its function children.

**Requirements:** coverage-collection#req:node-coverage-parsing, coverage-collection#req:metric-aggregation

### AC: canonical-tree-and-rollup

**Given** a collection run over several independent repositories

**When** the document is serialized

**Then** the tree is exactly root → repository → package → file → function, ids
match `repo:<rel>`, `pkg:<rel>`, `file:<rel>`, and `fn:<rel>#<name>@<line>`, no
line-level node exists, and every node carries all five `{covered, total}`
metric objects

**And** `lines`, `functions`, and `files` equal the element-wise sum of the
children, the package, repository, and root `packages` and `repositories` rules
hold as specified, siblings are ordered by descending `lines.total` then
`functions.total` then name, and concurrent collection of independent
repositories neither blocks nor corrupts another repository's result.

**Requirements:** coverage-collection#req:normalized-tree, coverage-collection#req:metric-aggregation, coverage-collection#req:project-detection

## Open Questions

- Should line-level nodes be materialized behind an explicit flag for users who
  want per-line drill-down, accepting the larger document?
- `go test -coverprofile ./...` does report packages that have no test files,
  with a zero count, but it cannot report a package excluded from `./...` by
  build tags or one that fails to build. Should `cover100` enumerate packages
  with `go list ./...` and synthesize the missing ones as fully uncovered, so a
  build failure cannot quietly shrink the denominator?
- Should branch coverage become a first-class metric in the normalized model, or
  remain an Istanbul-only extra carried on file nodes?

---
*This document follows the https://specscore.md/feature-specification*
