# Demo

## The 30-second version

```console
$ cd path/to/your/repo
$ cover100
```

```
cover100 0.1.0
info: scanning /Users/you/src/my-app

Detecting projects
ok: 1 Go module
ok: 1 Node package with a test script

Collecting coverage
ok: js  web (vitest)                                52.5% lines    21/40  727ms
ok: go  . (example.com/sample)                      27.3% lines     6/22  298ms

Report
repository    language    lines        functions     files
----------    --------    -----        ---------     -----
web           typescript  21/40 52.5%  4/8 50.0%     2/2 100.0%
sample-go-ts  go          6/22 27.3%   not measured  1/2 50.0%
overall                   27/62 43.5%  4/8 50.0%     3/4 75.0%

ok: wrote /Users/you/src/my-app/.cover100/coverage.json (15.5 KiB)
ok: wrote /Users/you/src/my-app/.cover100/view.html
info: serving http://127.0.0.1:5173/?data=/coverage.json&metric=lines&mode=percent
info: press Ctrl+C to stop
```

The browser opens on the treemap.

## What the screenshot shows

`treemap.png` in this directory is a headless capture of the real page, built
from `examples/coverage.example.json` — no mockups.

Reading it left to right:

1. **The top bar.** The metric dropdown, the `percent`/`count` toggle with
   `percent` active, the *size by* selector on `total`, one checkbox per
   language present in the report (`go`, `typescript`), the search box, a reset
   control, and the colour legend from 0% to 100%.
2. **The two repositories.** `web` (TypeScript) holds the larger share of the
   measured lines, so it takes the larger area; the Go module sits beside it.
3. **Colour.** `util/format.ts` is mostly uncovered and reads amber-to-red.
   `checkout/cart.ts` is more than half covered. The Go module's `internal/store`
   package is fully red — it has no tests at all.
4. **Grey is not red.** Selecting the **functions** metric greys out the Go
   boxes: Go profiles carry no function data, so those boxes say *not measured*
   rather than pretending to be 0%.

## Try the interactions

Once the page is open:

| Action | Result |
|---|---|
| Click a box | drills into that node; the breadcrumb tracks where you are |
| `Esc` | back out one level |
| Switch **metric** to `functions` | the map re-sizes and re-colours for that dimension, and the Go boxes grey out |
| Switch **mode** to `count` | tooltips lead with absolute `covered / total` instead of percentages |
| Set *size by* to `uncovered` | the map re-weights so the largest boxes are the largest problems, not the largest files |
| Uncheck `go` | the Go repository disappears and the remaining numbers are recomputed over just what is visible |
| Type in **search** | non-matching boxes dim, matches get an outline, and a match count appears |
| Hover a box | every metric for that node, its repository-relative path, and — for functions — the line number and hit count; ancestors get an outline |
| Right-click a box | copies its repository-relative path |

## Reproducing the screenshot

```console
$ make demo                       # regenerate examples/coverage.example.json
$ ./cover100 examples/sample-go-ts --no-serve --no-open --keep
$ open .cover100/view.html        # or serve it: ./cover100 examples/sample-go-ts
```
