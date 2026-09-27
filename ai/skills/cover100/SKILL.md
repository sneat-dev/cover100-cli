---
name: cover100
description: Collect test coverage across Go modules and Node/TypeScript packages, identify uncovered hot spots, analyze normalized coverage JSON, and visualize coverage with interactive treemaps.
---

# cover100

`cover100` collects test coverage from Go modules and Node/TypeScript packages, normalizes it into a single language-neutral JSON document, and provides an interactive zoomable treemap to inspect coverage metrics across repositories, packages, files, and functions.

## Agent Workflow: Headless Coverage Collection & Analysis

When running inside automated agent harnesses or CI pipelines, agents should execute `cover100` without launching a browser or starting a blocking server.

### 1. Collect Coverage to JSON

Run `cover100` with `--no-serve` to execute tests, aggregate coverage data, write the normalized report, and exit immediately:

```bash
cover100 --no-serve --out .cover100/coverage.json
```

To target a specific directory:

```bash
cover100 /path/to/project --no-serve --out .cover100/coverage.json
```

Useful collection flags:
- `--no-serve`: Write the report and exit without starting the local HTTP server.
- `--out <path>`: Path to write the JSON report (default: `.cover100/coverage.json`). Beside it, `view.html` is generated for offline browser viewing.
- `--lang go|ts|js|all`: Restrict coverage collection to specific languages (default: `all`).
- `--keep`: Retain intermediate collector files (e.g. Go `cover.out`, Istanbul `coverage-final.json`) under the output directory for debugging.
- `--timeout <duration>`: Timeout per project coverage command (default: `15m`, e.g. `--timeout 5m`).

### 2. Analyze Coverage JSON & Identify Hot Spots

The generated JSON file (`.cover100/coverage.json`) contains a hierarchical tree structure:
`root -> repository -> package -> file -> function`.

Each node in `tree` includes:
- `id`, `name`, `type` (`root`, `repository`, `package`, `file`, `function`)
- `path`: Relative path to file or package
- `language`: `go`, `typescript`, `javascript`, or `mixed`
- `lines`: `{"covered": <int>, "total": <int>}`
- `functions`: `{"covered": <int>, "total": <int>}` (TypeScript/JavaScript, or 0/0 if unmeasured in Go)
- `functionsAvailable`: boolean indicating if function-level coverage is available
- `branches`: `{"covered": <int>, "total": <int>}` (when available)
- `children`: Array of child nodes

#### Finding Uncovered Hot Spots
To find files with the lowest coverage or highest number of uncovered lines:

```bash
# Using jq to rank files by uncovered lines (greatest deficiency first)
jq '[.. | objects | select(.type=="file" and .lines.total > 0) | {path: .path, coverage_pct: ((.lines.covered / .lines.total) * 100), uncovered_lines: (.lines.total - .lines.covered), total_lines: .lines.total}] | sort_by(-.uncovered_lines)' .cover100/coverage.json
```

#### Finding Files Below Coverage Threshold
```bash
# Find all files with line coverage under 80%
jq '[.. | objects | select(.type=="file" and .lines.total > 0) | select((.lines.covered / .lines.total) < 0.8) | {path: .path, percent: ((.lines.covered / .lines.total) * 100 | round)}] | sort_by(.percent)' .cover100/coverage.json
```

Use these findings to prioritize which units or modules need additional tests.

---

## Interactive Visualization (Treemap)

When human developers want to explore coverage visually, `cover100` provides an interactive browser-based treemap where boxes are sized by metric volume and colored from red (0%) to amber (50%) to green (100%).

### Launching the Treemap

```bash
cover100 [path]
```

Treemap options:
- `--metric lines|functions|files|packages|repositories`: Metric the treemap opens with (default: `lines`).
- `--mode percent|count`: Colour mode to display (default: `percent`).
- `--port <number>`: Local HTTP server port (default: `5173`).
- `--no-open`: Start the server but do not automatically open the browser.
- `--file`: Open the self-contained `view.html` over `file://` instead of running a local server.

---

## Process & Browser Safety for Agents

When running tests or verifying `cover100` outputs:
- Do NOT kill browser processes with broad patterns like `pkill -f "Google Chrome"`.
- Use `--no-serve` to avoid hanging processes.
- If headless verification of `view.html` or the server is required, pass `--headless=new --no-sandbox --disable-gpu --use-mock-keychain --password-store=basic --user-data-dir=/tmp/chrome-cover100` to Chrome.

---

## Exit Codes

- `0`: Success — report was successfully generated and written.
- `2`: InvalidArgs — missing or invalid command-line flags/arguments.
- `3`: NotFound — target directory does not exist or contains no supported Go/Node projects.
- `10`: Unexpected — unrecoverable runtime failure (e.g. permission error, disk full).
