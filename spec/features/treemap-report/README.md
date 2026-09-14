---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Treemap report document and viewer

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/treemap-report?op=explore) | [Edit](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/treemap-report?op=edit) | [Ask question](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/treemap-report?op=ask) | [Request change](https://specscore.studio/app/github.com/sneat-dev/cover100-cli/spec/features/treemap-report?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Purpose

A treemap is only trustworthy if the rectangle area and the rectangle colour
mean what the legend says they mean. This feature fixes the contract of the
document the collector produces and of the page that renders it, so that every
rectangle is derived from a normalized metric, every colour from a stated scale,
and every filtered view from roll-ups recomputed over exactly what is visible.
The page has no backend: it is a static asset served by the CLI, and the same
sources must also produce a single file that works from `file://`.

### REQ: document-and-bootstrap

The report document MUST carry the top-level keys `generatedAt` (an RFC 3339
timestamp), `root` (the absolute scan path), `metrics`, `languages`, and `tree`.
It MUST additionally carry `warnings` and `tool`, which older consumers may
ignore. Additive keys, not new document versions, are how the tool evolves
without breaking a viewer shipped alongside an older collector.

`metrics` MUST be serialized in a canonical order so that repeated runs over the
same input produce identical bytes and diffs stay meaningful.

Every node in `tree` MUST carry `id`, `name`, `type` (one of
`root|repository|package|file|function`), `language` (one of
`go|typescript|javascript|mixed|null`), the five metric objects, and `children`,
plus the optional `path`, `line`, `hits`, `covered`, `functionsAvailable`, and
`branches` fields where they apply. `functionsAvailable` exists so that a
consumer can distinguish "no covered functions" from "function coverage was not
measurable", which Go profiles cannot provide.

The page MUST load its data from `window.__COVER100_DATA__` when present, so the
standalone `file://` case needs no network; otherwise from the URL given by
`?data=<url>`; otherwise from `coverage.json`. It MUST also honour `?metric=` and
`?mode=` as the initial metric and sizing mode, so that the CLI's defaults and
the UI's opening state cannot disagree.

Rendering MUST use D3 v7, loading `d3-hierarchy` and `d3.treemap` from
`https://cdn.jsdelivr.net/npm/d3@7`, and MUST lay out with `treemapSquarify`. If
D3 fails to load, the page MUST display a clear, visible error rather than a
blank canvas, because a silent blank page is indistinguishable from a report
with no data.

### REQ: treemap-sizing

Only leaf nodes MUST carry raw values; parent values MUST be summed by
`d3.hierarchy`, so that a container's area is always the sum of its children and
can never be hand-tuned.

In `count` mode the page MUST size by `node[metric].total`. In `percent` mode it
MUST size by `total` or by uncovered volume (`total - covered`) according to the
size-by toggle. If the selected metric sums to zero — the normal case for
`functions` on a Go-only tree — the page MUST fall back to `lines.total` and say
so visibly, instead of rendering an empty canvas that a user would misread as
zero coverage.

### REQ: coverage-colour-scale

Colour MUST be produced by
`d3.scaleLinear().domain([0, 0.5, 1]).range(['#e74c3c', '#f1c40f', '#2ecc71'])`
keyed on the node's percentage for the selected metric. A node whose metric pair
has `total === 0` MUST be drawn in a neutral grey and labelled explicitly as
"not measured".

Colour MUST NOT be keyed off the sizing value. Size answers "how large is this
area of code", colour answers "how well is it covered"; conflating them would
make the uncovered-volume view colour every large file red regardless of its
actual coverage.

### REQ: viewer-controls

The page MUST provide a metric dropdown, a percent/count toggle, a size-by
dropdown, per-language checkboxes driven by the document's `languages`, a search
box, a clickable breadcrumb, a legend whose 0–100% labels are built from the
same colour scale, a reset control, and an overall summary for the current focus
and filter. Each control MUST be driven by the document rather than by a
hard-coded language list, so that a report containing no Go or no TypeScript
still renders coherent controls.

Search MUST dim non-matching nodes, outline matching nodes, and display a live
match count. A search that silently hides matches or shows no count leaves the
user unable to tell "no results" from "broken filter".

Hovering a node MUST show a tooltip containing the name, type, language, the path
relative to the repository root, every metric as `covered / total (xx.x%)` or
"not measured", the function line and hit count, a copy-path control, and the Go
function-coverage caveat. In `count` mode the tooltip MUST emphasise absolute
counts; in `percent` mode it MUST emphasise percentages, so the tooltip agrees
with the view the user selected.

### REQ: filter-aware-rollup

The language filter MUST prune the tree, and the page MUST re-derive roll-ups
over the visible subtree using the same aggregation rules as the collector. A
page that shows a repository's unfiltered totals next to its filtered children
is a defect: the numbers would not add up, and the percentage shown for a
container would describe code the user explicitly filtered out.

### REQ: interaction-and-layout

Clicking a node MUST drill into it; `Esc` MUST zoom out exactly one level;
hovering MUST outline the ancestors of the hovered node; right-click MUST copy
the node's relative path; and the treemap MUST re-lay-out on resize and fill the
viewport below the top bar. Exactly-one-level zoom-out keeps `Esc` a predictable
undo for drill-down rather than a jump to the root.

The page MUST use a dark background of `#1e1e1e`, a system UI font stack, and a
responsive layout, because the report is a local developer tool that must be
readable immediately without a theme setup step.

### REQ: standalone-document

The CLI MUST be able to write a single self-contained HTML file containing the
CSS, the JavaScript, and the JSON, because `fetch()` from a `file://` origin is
blocked and an offline share of a report must still work with no server.

The served page and the standalone page MUST be generated from the same sources.
Two hand-maintained variants would drift, and a diverged standalone page would
misreport exactly the numbers it exists to preserve.

## Acceptance criteria

### AC: report-schema

**Given** a completed collection run producing a coverage document

**When** the document is inspected

**Then** `generatedAt` parses as RFC 3339, `root` is the absolute scan path,
`metrics`, `languages`, and `tree` are present in canonical order, and `warnings`
and `tool` are present as additive keys

**And** every node carries `id`, `name`, `type`, `language`, the five metric
objects, and `children`, with `path`, `line`, `hits`, `covered`,
`functionsAvailable`, and `branches` populated where they apply.

**Requirements:** treemap-report#req:document-and-bootstrap

### AC: data-loading-and-renderer-failure

**Given** the standalone document, a served document addressed by `?data=<url>`,
a served document with no query, and a build in which the D3 CDN script fails

**When** the page loads under each condition

**Then** it reads `window.__COVER100_DATA__` when present, else the `?data=` URL,
else `coverage.json`, and it applies `?metric=` and `?mode=` as the initial state

**And** in the D3-failure case it shows a clear visible error instead of a blank
canvas, while layout uses `d3.treemap` with `treemapSquarify` from the pinned
D3 v7 `jsdelivr` URL.

**Requirements:** treemap-report#req:document-and-bootstrap

### AC: sizing-modes-and-zero-fallback

**Given** a tree with measured metrics, a selected metric that sums to zero, and
the percent/count and size-by controls

**When** the user switches modes and sizes by uncovered volume

**Then** only leaves carry raw values and parents are summed by d3, `count` mode
sizes by `node[metric].total`, and `percent` mode sizes by `total` or by
`total - covered` per the size-by toggle

**And** a zero-sum selected metric falls back to `lines.total` with a visible
statement of the fallback rather than an empty canvas.

**Requirements:** treemap-report#req:treemap-sizing

### AC: colour-encoding

**Given** nodes at 0%, 50%, and 100% for the selected metric, and a node whose
metric total is 0

**When** the treemap is drawn

**Then** the first three map through
`domain([0, 0.5, 1]).range(['#e74c3c', '#f1c40f', '#2ecc71'])`, the zero-total
node is neutral grey and labelled "not measured", and the legend's 0–100% labels
are generated from that same scale

**And** changing the size-by toggle changes rectangle areas without changing any
rectangle's colour.

**Requirements:** treemap-report#req:coverage-colour-scale

### AC: filter-aware-rollup

**Given** a multi-language tree and a language filter that excludes some
languages

**When** the filter is applied

**Then** the tree is pruned to the visible languages and every container's
`lines`, `functions`, `files`, `packages`, and `repositories` are recomputed over
the visible subtree using the collector's aggregation rules

**And** no container displays an unfiltered total alongside filtered children.

**Requirements:** treemap-report#req:filter-aware-rollup, treemap-report#req:viewer-controls

### AC: controls-and-tooltip

**Given** a rendered report

**When** the user operates the metric dropdown, percent/count toggle, size-by
dropdown, language checkboxes, search box, breadcrumb, legend, and reset control

**Then** each control changes the view as labelled, the language checkboxes are
driven by the document's `languages`, search dims non-matches, outlines matches,
and shows a live match count, and the summary reflects the current focus and
filter

**And** hovering shows a tooltip with name, type, language, repository-relative
path, every metric as `covered / total (xx.x%)` or "not measured", the function
line and hit count, a copy-path control, and the Go caveat, emphasising absolute
counts in `count` mode and percentages in `percent` mode.

**Requirements:** treemap-report#req:viewer-controls

### AC: interaction-and-standalone-parity

**Given** a rendered treemap and a generated standalone HTML file

**When** the user clicks a node, presses `Esc`, hovers, right-clicks, and resizes
the window, and when the standalone file is opened directly from `file://`

**Then** click drills in, `Esc` zooms out exactly one level, hover outlines
ancestors, right-click copies the relative path, the treemap re-lays-out on
resize and fills the viewport below the top bar, and the standalone file renders
and interacts with no server and no `fetch()`

**And** the standalone page and the served page are produced from the same
sources, and both use the `#1e1e1e` dark background, the system UI font stack,
and a responsive layout.

**Requirements:** treemap-report#req:interaction-and-layout, treemap-report#req:standalone-document

## Open Questions

- Should the standalone file be able to embed a second dataset, such as a
  baseline run, so that two revisions can be compared in one page?
- Should the viewer expose a per-function list as an alternative to the treemap
  for repositories whose many small files make a treemap hard to read?
- Should the language filter support exclude semantics as well as include, or is
  a checkbox per language sufficient?

---
*This document follows the https://specscore.md/feature-specification*
