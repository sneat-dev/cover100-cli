// Package tscov collects JavaScript and TypeScript coverage by running the
// package's own test runner with a JSON coverage reporter, then parsing the
// Istanbul `coverage-final.json` (or, as a fallback, `lcov.info`) it produces.
package tscov

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/sneat-dev/cover100-cli/internal/detect"
	"github.com/sneat-dev/cover100-cli/internal/model"
	"github.com/sneat-dev/cover100-cli/internal/pathutil"
	"github.com/sneat-dev/cover100-cli/internal/ui"
)

const (
	runnerJest   = "jest"
	runnerVitest = "vitest"

	maxOutputBytes = 256 << 10
)

// Options configures a collection run.
type Options struct {
	// Root is the absolute scan root, used to make reported paths relative.
	Root string
	// WorkDir receives one coverage directory per package.
	WorkDir string
	// Printer receives progress and diagnostics.
	Printer *ui.Printer
}

// Result is the outcome for one Node package. A Result is always returned,
// even when the runner failed: cover100 reports what it can and records the
// failure as a warning instead of discarding the package.
type Result struct {
	Dir      string
	Rel      string
	Runner   string
	Files    []model.FileCoverage
	Warnings []string
	Command  string
	ExitCode int
	Output   string
	Err      error
}

// Run executes the package's tests with coverage and parses the report.
func Run(ctx context.Context, p detect.Project, opts Options, index int) *Result {
	res := &Result{Dir: p.Dir, Rel: p.Rel, Runner: p.Runner}

	runner := p.Runner
	if runner == "" {
		// Jest is the historical default and the reporter flags below are
		// jest-compatible; a package that reaches here has a test script but no
		// recognisable runner dependency.
		runner = runnerJest
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: no test runner dependency found; invoking %s", p.Rel, runner))
	}
	res.Runner = runner

	outDir := filepath.Join(opts.WorkDir, fmt.Sprintf("ts-%d", index))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		res.Err = err
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: creating coverage directory: %v", p.Rel, err))
		return res
	}

	args := runnerArgs(runner, outDir)
	bin, ok := findRunnerBinary(p.Dir, runner)
	if ok {
		res.Command = bin + " " + strings.Join(args, " ")
	} else {
		res.Command = "npx -y " + runner + " " + strings.Join(args, " ")
	}

	if opts.Printer != nil {
		opts.Printer.Debugf("%s: running %s", p.Rel, res.Command)
	}

	var cmd *exec.Cmd
	if ok {
		cmd = exec.CommandContext(ctx, bin, args...)
	} else {
		npx, err := exec.LookPath("npx")
		if err != nil {
			res.Err = err
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"%s: neither a local %s nor npx on PATH; skipping (install Node.js or run npm install first)",
				p.Rel, runner))
			return res
		}
		cmd = exec.CommandContext(ctx, npx, append([]string{"-y", runner}, args...)...)
	}
	cmd.Dir = p.Dir
	// CI keeps vitest out of watch mode and makes jest's output non-interactive.
	cmd.Env = append(os.Environ(), "CI=true")

	output, err := cmd.CombinedOutput()
	res.Output = truncate(string(output), maxOutputBytes)
	if err != nil {
		res.Err = err
		res.ExitCode = exitCode(err)
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: %s reported test failures (exit %d); parser will use whatever coverage was written",
			p.Rel, runner, res.ExitCode))
	}

	reportPath, kind := findReport(outDir)
	if reportPath == "" {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: no coverage report was produced in %s; check that the %s coverage provider is installed",
			p.Rel, outDir, runner))
		return res
	}
	if opts.Printer != nil {
		opts.Printer.Debugf("%s: parsing %s (%s)", p.Rel, reportPath, kind)
	}

	data, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		res.Err = readErr
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: reading %s: %v", p.Rel, reportPath, readErr))
		return res
	}

	var (
		files    []model.FileCoverage
		warnings []string
		parseErr error
	)
	if kind == "istanbul" {
		files, warnings, parseErr = ParseIstanbul(data, opts.Root)
	} else {
		files, warnings, parseErr = ParseLcov(data, opts.Root)
	}
	res.Warnings = append(res.Warnings, warnings...)
	if parseErr != nil {
		// A parse failure replaces the runner's own error only because it
		// explains why no coverage could be read at all; a successful parse
		// must leave a failing test command recorded.
		res.Err = parseErr
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: parsing %s: %v", p.Rel, filepath.Base(reportPath), parseErr))
		res.Files = nil
		return res
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	res.Files = files
	return res
}

// runnerArgs builds the coverage flags for a runner. Both write an
// Istanbul-format report the parser understands.
func runnerArgs(runner, outDir string) []string {
	if runner == runnerVitest {
		return []string{
			"run",
			"--coverage.enabled=true",
			"--coverage.reporter=json",
			"--coverage.reportsDirectory=" + outDir,
		}
	}
	return []string{
		"--coverage",
		"--coverageReporters=json",
		"--coverageDirectory=" + outDir,
	}
}

// findRunnerBinary looks for a locally installed runner, walking up from the
// package directory so hoisted monorepo installs are found too.
func findRunnerBinary(dir, runner string) (string, bool) {
	name := runner
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	for d := dir; ; {
		candidate := filepath.Join(d, "node_modules", ".bin", name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// findReport locates the coverage artefact, preferring the richer Istanbul
// report and searching nested directories because reporters may create a
// per-run subdirectory.
func findReport(outDir string) (path, kind string) {
	if p, ok := findNamed(outDir, "coverage-final.json"); ok {
		return p, "istanbul"
	}
	if p, ok := findNamed(outDir, "lcov.info"); ok {
		return p, "lcov"
	}
	return "", ""
}

func findNamed(dir, name string) (string, bool) {
	direct := filepath.Join(dir, name)
	if info, err := os.Stat(direct); err == nil && !info.IsDir() {
		return direct, true
	}
	var found string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil
		}
		if d.Name() == name {
			found = p
		}
		return nil
	})
	return found, found != ""
}

// --- Istanbul (coverage-final.json) ---

type istanbulPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type istanbulLocation struct {
	Start istanbulPosition `json:"start"`
	End   istanbulPosition `json:"end"`
}

type istanbulFunction struct {
	Name string           `json:"name"`
	Decl istanbulLocation `json:"decl"`
	Loc  istanbulLocation `json:"loc"`
	Line int              `json:"line"`
}

type istanbulBranch struct {
	Loc       istanbulLocation   `json:"loc"`
	Type      string             `json:"type"`
	Locations []istanbulLocation `json:"locations"`
}

type istanbulFile struct {
	Path         string                      `json:"path"`
	StatementMap map[string]istanbulLocation `json:"statementMap"`
	FnMap        map[string]istanbulFunction `json:"fnMap"`
	BranchMap    map[string]istanbulBranch   `json:"branchMap"`
	S            map[string]int              `json:"s"`
	F            map[string]int              `json:"f"`
	B            map[string][]int            `json:"b"`
	// L is the line-coverage map some Istanbul providers emit directly. When
	// present it is authoritative; when absent it is derived from statements.
	L map[string]int `json:"l"`
}

// ParseIstanbul parses an Istanbul coverage-final.json document. Paths inside
// the document are absolute; files outside root are skipped.
func ParseIstanbul(data []byte, root string) ([]model.FileCoverage, []string, error) {
	var raw map[string]istanbulFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}

	paths := make([]string, 0, len(raw))
	for path := range raw {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var (
		files    []model.FileCoverage
		warnings []string
		skipped  int
	)
	for _, path := range paths {
		entry := raw[path]
		abs := entry.Path
		if abs == "" {
			abs = path
		}
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(filepath.Dir(path), abs)
		}
		rel, ok := pathutil.RelTo(root, abs)
		if !ok {
			skipped++
			continue
		}
		language, ok := languageOf(rel)
		if !ok {
			skipped++
			continue
		}

		lines := lineCoverage(entry)
		funcs := functionCoverage(entry)

		file := model.FileCoverage{
			Path:               rel,
			Language:           language,
			Lines:              metricOf(lines),
			FunctionsAvailable: true,
			Functions:          model.Metric{Covered: countCoveredFuncs(funcs), Total: len(funcs)},
			FunctionList:       funcs,
		}
		if branch, ok := branchMetric(entry); ok {
			file.Branches = &branch
		}
		files = append(files, file)
	}
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"ignored %d coverage entries outside the scan root or of an unsupported file type", skipped))
	}
	return files, warnings, nil
}

// lineCoverage resolves per-line hit counts.
//
// Istanbul's own line coverage attributes a statement to the line it starts
// on and keeps the highest count when several statements share a line, which
// is reproduced here for reports that carry no `l` map.
func lineCoverage(f istanbulFile) map[int]int {
	lines := make(map[int]int, len(f.L))
	if len(f.L) > 0 {
		for key, hits := range f.L {
			if n, err := strconv.Atoi(key); err == nil {
				lines[n] = hits
			}
		}
		return lines
	}
	for id, loc := range f.StatementMap {
		line := loc.Start.Line
		if line <= 0 {
			continue
		}
		// A never-executed statement still declares its line: recording only
		// hits > 0 would drop it from the denominator entirely.
		if prev, ok := lines[line]; !ok || f.S[id] > prev {
			lines[line] = f.S[id]
		}
	}
	return lines
}

// functionCoverage builds one entry per function, attributing to it the
// statements it owns. A statement belongs to the innermost function whose
// location contains the statement's start position; top-level statements
// belong to the file alone and are therefore counted by no function.
func functionCoverage(f istanbulFile) []model.FuncCoverage {
	if len(f.FnMap) == 0 {
		return nil
	}

	type fnEntry struct {
		id  string
		fn  istanbulFunction
		loc istanbulLocation
	}
	ids := make([]string, 0, len(f.FnMap))
	for id := range f.FnMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	entries := make([]fnEntry, 0, len(ids))
	for _, id := range ids {
		fn := f.FnMap[id]
		loc := fn.Loc
		if loc.Start.Line <= 0 {
			loc = fn.Decl
		}
		entries = append(entries, fnEntry{id: id, fn: fn, loc: loc})
	}

	owned := make(map[string]map[int]int, len(entries))
	stmtIDs := make([]string, 0, len(f.StatementMap))
	for id := range f.StatementMap {
		stmtIDs = append(stmtIDs, id)
	}
	sort.Strings(stmtIDs)

	for _, id := range stmtIDs {
		pos := f.StatementMap[id].Start
		owner, ownerSpan := "", 0
		for _, e := range entries {
			if e.loc.Start.Line <= 0 || !locationContains(e.loc, pos) {
				continue
			}
			span := e.loc.End.Line - e.loc.Start.Line
			if owner == "" || span < ownerSpan || (span == ownerSpan && e.id < owner) {
				owner, ownerSpan = e.id, span
			}
		}
		if owner == "" {
			continue
		}
		lines := owned[owner]
		if lines == nil {
			lines = make(map[int]int)
			owned[owner] = lines
		}
		line := pos.Line
		// Uncovered statements must still claim their line, so a function that
		// never ran reports "0 of N lines" rather than 0 of 0.
		if prev, ok := lines[line]; !ok || f.S[id] > prev {
			lines[line] = f.S[id]
		}
	}

	out := make([]model.FuncCoverage, 0, len(entries))
	for i, e := range entries {
		name := e.fn.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("(anonymous_%d)", i+1)
		}
		line := e.fn.Decl.Start.Line
		if line <= 0 {
			line = e.loc.Start.Line
		}
		if line <= 0 {
			line = e.fn.Line
		}
		hits := f.F[e.id]
		out = append(out, model.FuncCoverage{
			Name:    name,
			Line:    line,
			Hits:    hits,
			Covered: hits > 0,
			Lines:   metricOf(owned[e.id]),
		})
	}
	// Deterministic order: by declaration line, then name.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func locationContains(outer istanbulLocation, pos istanbulPosition) bool {
	return positionLE(outer.Start, pos) && positionLE(pos, outer.End)
}

func positionLE(a, b istanbulPosition) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column <= b.Column
}

func branchMetric(f istanbulFile) (model.Metric, bool) {
	if len(f.BranchMap) == 0 {
		return model.Metric{}, false
	}
	total, covered := 0, 0
	for id := range f.BranchMap {
		hits := f.B[id]
		if len(hits) == 0 {
			// A branch declared but never instrumented still counts toward the
			// denominator, otherwise a fully untested file would look perfect.
			total++
			continue
		}
		for _, h := range hits {
			total++
			if h > 0 {
				covered++
			}
		}
	}
	return model.Metric{Covered: covered, Total: total}, true
}

// --- lcov ---

// ParseLcov parses an lcov.info document. It is the fallback when a runner
// wrote only the lcov reporter.
//
// lcov records a function's declaration line and hit count, but no body range,
// so functions parsed from lcov report lines as unmeasured rather than
// attributing arbitrary lines to them.
func ParseLcov(data []byte, root string) ([]model.FileCoverage, []string, error) {
	type lcovFunc struct {
		name    string
		line    int
		hits    int
		matched bool
	}
	type record struct {
		path      string
		lines     map[int]int
		funcs     []*lcovFunc
		branchN   int
		branchHit int
	}

	var (
		files    []model.FileCoverage
		warnings []string
		skipped  int
		current  *record
	)

	flush := func() {
		if current == nil || current.path == "" {
			current = nil
			return
		}
		defer func() { current = nil }()

		rel, ok := pathutil.RelTo(root, current.path)
		if !ok {
			skipped++
			return
		}
		language, ok := languageOf(rel)
		if !ok {
			skipped++
			return
		}

		file := model.FileCoverage{
			Path:               rel,
			Language:           language,
			Lines:              metricOf(current.lines),
			FunctionsAvailable: true,
		}
		covered := 0
		for _, fn := range current.funcs {
			if fn.hits > 0 {
				covered++
			}
			file.FunctionList = append(file.FunctionList, model.FuncCoverage{
				Name:    fn.name,
				Line:    fn.line,
				Hits:    fn.hits,
				Covered: fn.hits > 0,
			})
		}
		file.Functions = model.Metric{Covered: covered, Total: len(current.funcs)}
		if current.branchN > 0 {
			branch := model.Metric{Covered: current.branchHit, Total: current.branchN}
			file.Branches = &branch
		}
		files = append(files, file)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "end_of_record":
			flush()
		case strings.HasPrefix(line, "SF:"):
			flush()
			current = &record{path: strings.TrimPrefix(line, "SF:"), lines: make(map[int]int)}
		case current == nil:
			continue
		case strings.HasPrefix(line, "DA:"):
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) < 2 {
				continue
			}
			n, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			hits, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			// DA: records with a zero count are still lines that must be
			// reported as uncovered.
			if prev, ok := current.lines[n]; !ok || hits > prev {
				current.lines[n] = hits
			}
		case strings.HasPrefix(line, "FN:"):
			parts := strings.SplitN(strings.TrimPrefix(line, "FN:"), ",", 2)
			if len(parts) != 2 {
				continue
			}
			n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				continue
			}
			current.funcs = append(current.funcs, &lcovFunc{name: strings.TrimSpace(parts[1]), line: n})
		case strings.HasPrefix(line, "FNDA:"):
			parts := strings.SplitN(strings.TrimPrefix(line, "FNDA:"), ",", 2)
			if len(parts) != 2 {
				continue
			}
			hits, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				continue
			}
			name := strings.TrimSpace(parts[1])
			// FNDA lines follow their FN lines, but names repeat across
			// scopes, so match the first still-unmatched entry with that name.
			for _, fn := range current.funcs {
				if fn.name == name && !fn.matched {
					fn.hits, fn.matched = hits, true
					break
				}
			}
		case strings.HasPrefix(line, "BRDA:"):
			parts := strings.Split(strings.TrimPrefix(line, "BRDA:"), ",")
			if len(parts) < 4 {
				continue
			}
			current.branchN++
			if taken := strings.TrimSpace(parts[3]); taken != "-" && taken != "0" {
				current.branchHit++
			}
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, warnings, err
	}
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"ignored %d lcov records outside the scan root or of an unsupported file type", skipped))
	}
	return files, warnings, nil
}

// --- shared helpers ---

// languageOf classifies a path by extension. Unsupported extensions are
// skipped rather than guessed at, so a coverage report's JSON or CSS entries
// never become coverage nodes.
func languageOf(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".mts", ".cts":
		return model.LangTypeScript, true
	case ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte", ".astro":
		return model.LangJavaScript, true
	default:
		return "", false
	}
}

func countCoveredFuncs(funcs []model.FuncCoverage) int {
	n := 0
	for _, f := range funcs {
		if f.Covered {
			n++
		}
	}
	return n
}

func metricOf(lines map[int]int) model.Metric {
	total, covered := len(lines), 0
	for _, hits := range lines {
		if hits > 0 {
			covered++
		}
	}
	return model.Metric{Covered: covered, Total: total}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (output truncated)"
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
