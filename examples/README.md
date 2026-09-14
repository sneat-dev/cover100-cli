# Examples

## `sample-go-ts/`

A deliberately small project with one Go module and one TypeScript package, so
that a single `cover100` run exercises both collectors and produces a treemap
with a real spread of colours rather than a uniformly green field.

```
sample-go-ts/
├── go.mod                  # module example.com/sample
├── calc/
│   ├── calc.go             # Add/Sub/Div covered, Mul/Describe deliberately not
│   └── calc_test.go
├── internal/store/
│   └── store.go            # no test file at all: `go test` reports it as 0%
└── web/
    ├── package.json        # has a test script, so cover100 runs it
    ├── vitest.config.ts
    └── src/
        ├── checkout/cart.ts       # addItem/total covered, removeItem/discount not
        ├── checkout/cart.test.ts
        ├── util/format.ts         # formatMoney covered, formatPercent/truncate not
        └── util/format.test.ts
```

What each part is there to demonstrate:

| Fixture | Demonstrates |
|---|---|
| `calc` | a partially covered Go package |
| `internal/store` | a Go package with no tests, reported as 0% because `go test -coverprofile ./...` still instruments it |
| `checkout/cart.ts` | a partially covered TypeScript file with several functions, so the **functions** metric is meaningful |
| `util/format.ts` | an almost entirely uncovered file, so the map has a red box |
| `calc_test.go` / `*.test.ts` | test files are excluded from the report even though Go and Istanbul both report on them |

Try it:

```console
$ npm install --prefix examples/sample-go-ts/web
$ cover100 examples/sample-go-ts
```

The package.json without a `test` script is not represented here on purpose —
add one and cover100 will skip it with a note rather than guessing how to run it.

## `coverage.example.json`

The committed output of a run over `sample-go-ts`, so the report format can be
read without installing anything:

```console
$ python3 -m json.tool examples/coverage.example.json | head -40
```

It reports **14 / 42 lines (33.3%)** overall and **5 / 10 functions (50.0%)**,
with Go function coverage correctly shown as `{covered: 0, total: 0}`.

The TypeScript half of those numbers depends on the coverage provider's own
granularity and will move when vitest does: vitest's v8 provider reports
statement-level lines (`cart.ts` → 15 lines), while older versions emitted a
coarser line map for the same file (28 lines). The Go half does not move,
because it comes from `go test` itself. Regenerating after a dependency bump is
therefore expected to change this file's numbers — that is the provider being
more or less precise, not cover100 measuring differently.

Its `root` field is `/tmp/cover100-example/sample-go-ts` — the neutral scratch
path the generator used, not the machine that produced it. Regenerate it with:

```console
$ make demo
```

`scripts/demo.sh` copies the fixture to a scratch directory, installs its
dependencies, runs cover100 against the copy with `--no-serve --no-open`, and
writes the result here. Node and npm are required for the fixture's vitest run.
