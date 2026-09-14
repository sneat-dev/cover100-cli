package tscov

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/model"
)

// istanbulDocument builds a coverage-final.json body for one file with three
// statements (two covered, one not) spread over two functions.
func istanbulDocument(absPath string) string {
	return fmt.Sprintf(`{
  %q: {
    "path": %q,
    "statementMap": {
      "0": {"start": {"line": 1, "column": 0}, "end": {"line": 1, "column": 10}},
      "1": {"start": {"line": 2, "column": 0}, "end": {"line": 2, "column": 10}},
      "2": {"start": {"line": 5, "column": 0}, "end": {"line": 5, "column": 10}}
    },
    "fnMap": {
      "0": {"name": "coveredFn", "decl": {"start": {"line": 1, "column": 0}, "end": {"line": 1, "column": 9}},
            "loc": {"start": {"line": 1, "column": 0}, "end": {"line": 2, "column": 11}}, "line": 1},
      "1": {"name": "uncoveredFn", "decl": {"start": {"line": 5, "column": 0}, "end": {"line": 5, "column": 9}},
            "loc": {"start": {"line": 5, "column": 0}, "end": {"line": 6, "column": 1}}, "line": 5}
    },
    "branchMap": {
      "0": {"loc": {"start": {"line": 2, "column": 0}, "end": {"line": 2, "column": 10}},
            "type": "if", "locations": [
              {"start": {"line": 2, "column": 0}, "end": {"line": 2, "column": 5}},
              {"start": {"line": 2, "column": 5}, "end": {"line": 2, "column": 10}}]}
    },
    "s": {"0": 1, "1": 1, "2": 0},
    "f": {"0": 3, "1": 0},
    "b": {"0": [1, 0]}
  }
}`, absPath, absPath)
}

func TestParseIstanbul_CountsUncoveredStatements(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "src", "a.ts")

	files, warnings, err := ParseIstanbul([]byte(istanbulDocument(abs)), root)
	if err != nil {
		t.Fatalf("ParseIstanbul() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(files) != 1 {
		t.Fatalf("parsed %d files, want 1", len(files))
	}

	got := files[0]
	if got.Path != "src/a.ts" {
		t.Errorf("Path = %q, want %q", got.Path, "src/a.ts")
	}
	if got.Language != model.LangTypeScript {
		t.Errorf("Language = %q, want %q", got.Language, model.LangTypeScript)
	}
	// Regression guard: an uncovered statement must still contribute to the
	// denominator. Recording only hits > 0 made every file look 100% covered.
	if want := (model.Metric{Covered: 2, Total: 3}); got.Lines != want {
		t.Errorf("Lines = %+v, want %+v", got.Lines, want)
	}
	if want := (model.Metric{Covered: 1, Total: 2}); got.Functions != want {
		t.Errorf("Functions = %+v, want %+v", got.Functions, want)
	}
	if !got.FunctionsAvailable {
		t.Error("FunctionsAvailable = false, want true for Istanbul reports")
	}
	if got.Branches == nil {
		t.Fatal("Branches = nil, want a branch metric")
	}
	if want := (model.Metric{Covered: 1, Total: 2}); *got.Branches != want {
		t.Errorf("Branches = %+v, want %+v", *got.Branches, want)
	}
}

func TestParseIstanbul_AttributesStatementsToInnermostFunction(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "src", "a.ts")

	files, _, err := ParseIstanbul([]byte(istanbulDocument(abs)), root)
	if err != nil {
		t.Fatalf("ParseIstanbul() error = %v", err)
	}
	byName := map[string]model.FuncCoverage{}
	for _, fn := range files[0].FunctionList {
		byName[fn.Name] = fn
	}

	covered, ok := byName["coveredFn"]
	if !ok {
		t.Fatalf("coveredFn missing from %v", files[0].FunctionList)
	}
	if want := (model.Metric{Covered: 2, Total: 2}); covered.Lines != want {
		t.Errorf("coveredFn.Lines = %+v, want %+v", covered.Lines, want)
	}
	if !covered.Covered || covered.Hits != 3 || covered.Line != 1 {
		t.Errorf("coveredFn = %+v, want covered with 3 hits at line 1", covered)
	}

	uncovered := byName["uncoveredFn"]
	if want := (model.Metric{Covered: 0, Total: 1}); uncovered.Lines != want {
		t.Errorf("uncoveredFn.Lines = %+v, want %+v (its statement must not vanish)", uncovered.Lines, want)
	}
	if uncovered.Covered {
		t.Error("uncoveredFn.Covered = true, want false")
	}
}

func TestParseIstanbul_SkipsFilesOutsideRootAndUnknownTypes(t *testing.T) {
	root := t.TempDir()
	body := fmt.Sprintf(`{
	  %q: {"path": %q, "statementMap": {}, "fnMap": {}, "branchMap": {}, "s": {}, "f": {}, "b": {}},
	  %q: {"path": %q, "statementMap": {}, "fnMap": {}, "branchMap": {}, "s": {}, "f": {}, "b": {}}
	}`,
		filepath.Join(root, "..", "outside.ts"), filepath.Join(root, "..", "outside.ts"),
		filepath.Join(root, "styles.css"), filepath.Join(root, "styles.css"))

	files, warnings, err := ParseIstanbul([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseIstanbul() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("parsed %d files, want 0", len(files))
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want exactly one summary warning", warnings)
	}
}

func TestParseLcov(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "b.ts")
	body := "TN:\n" +
		"SF:" + src + "\n" +
		"FN:1,alpha\n" +
		"FN:4,beta\n" +
		"FNDA:2,alpha\n" +
		"FNDA:0,beta\n" +
		"DA:1,2\n" +
		"DA:2,0\n" +
		"DA:4,0\n" +
		"BRDA:2,0,0,1\n" +
		"BRDA:2,0,1,-\n" +
		"LF:3\nLH:1\nend_of_record\n"

	files, warnings, err := ParseLcov([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseLcov() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(files) != 1 {
		t.Fatalf("parsed %d files, want 1", len(files))
	}

	got := files[0]
	if want := (model.Metric{Covered: 1, Total: 3}); got.Lines != want {
		t.Errorf("Lines = %+v, want %+v", got.Lines, want)
	}
	if want := (model.Metric{Covered: 1, Total: 2}); got.Functions != want {
		t.Errorf("Functions = %+v, want %+v", got.Functions, want)
	}
	if got.Branches == nil || (model.Metric{Covered: 1, Total: 2}) != *got.Branches {
		t.Errorf("Branches = %+v, want 1/2 (an untaken branch still counts)", got.Branches)
	}
	// lcov records no function body range, so function line counts are left
	// unmeasured rather than guessed.
	for _, fn := range got.FunctionList {
		if fn.Lines.Total != 0 {
			t.Errorf("%s.Lines = %+v, want 0/0 for lcov", fn.Name, fn.Lines)
		}
	}
}

func TestRunnerArgs(t *testing.T) {
	jest := runnerArgs(runnerJest, "/tmp/out")
	if len(jest) == 0 || jest[1] != "--coverageReporters=json" {
		t.Errorf("jest args = %v", jest)
	}
	vitest := runnerArgs(runnerVitest, "/tmp/out")
	if len(vitest) != 4 || vitest[0] != "run" || vitest[2] != "--coverage.reporter=json" {
		t.Errorf("vitest args = %v", vitest)
	}
}

func TestLanguageOf(t *testing.T) {
	tests := map[string]string{
		"a.ts": model.LangTypeScript, "a.tsx": model.LangTypeScript, "a.mts": model.LangTypeScript,
		"a.js": model.LangJavaScript, "a.jsx": model.LangJavaScript, "a.mjs": model.LangJavaScript,
		"a.vue": model.LangJavaScript, "a.svelte": model.LangJavaScript,
	}
	for name, want := range tests {
		got, ok := languageOf(name)
		if !ok || got != want {
			t.Errorf("languageOf(%q) = (%q, %v), want (%q, true)", name, got, ok, want)
		}
	}
	for _, name := range []string{"a.json", "a.css", "a"} {
		if _, ok := languageOf(name); ok {
			t.Errorf("languageOf(%q) reported a language, want unsupported", name)
		}
	}
}

// --- findRunnerBinary ---

func TestFindRunnerBinary_FindsHoistedAncestorInstall(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(binDir, "vitest")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The package sits below the workspace root, where node_modules was
	// hoisted; the walk must climb out of the package directory to find it.
	pkgDir := filepath.Join(root, "packages", "web")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := findRunnerBinary(pkgDir, "vitest")
	if !ok {
		t.Fatalf("findRunnerBinary(%q, vitest) = not found, want the hoisted install at %q", pkgDir, want)
	}
	if got != want {
		t.Errorf("findRunnerBinary() = %q, want %q", got, want)
	}
}

func TestFindRunnerBinary_ReportsAbsentRunnerAtFilesystemRoot(t *testing.T) {
	dir := t.TempDir()

	got, ok := findRunnerBinary(dir, "cover100-absent-runner-3d9c1")
	if ok {
		t.Errorf("findRunnerBinary(%q, absent) = (%q, true), want not found", dir, got)
	}
	if got != "" {
		t.Errorf("findRunnerBinary() path = %q, want empty when the runner is absent", got)
	}
}

// --- findNamed ---

func TestFindNamed_FindsNestedReport(t *testing.T) {
	outDir := t.TempDir()
	nested := filepath.Join(outDir, "coverage", "run-1")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(nested, "coverage-final.json")
	if err := os.WriteFile(want, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := findNamed(outDir, "coverage-final.json")
	if !ok {
		t.Fatalf("findNamed(%q) did not find the report nested in %q", outDir, nested)
	}
	if got != want {
		t.Errorf("findNamed() = %q, want %q", got, want)
	}
}

func TestFindNamed_ReportsMissingReport(t *testing.T) {
	outDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outDir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := findNamed(outDir, "coverage-final.json")
	if ok {
		t.Errorf("findNamed() = (%q, true), want not found", got)
	}
	if got != "" {
		t.Errorf("findNamed() path = %q, want empty when nothing matches", got)
	}
}

// --- ParseIstanbul ---

func TestParseIstanbul_ErrorsOnMalformedJSON(t *testing.T) {
	_, _, err := ParseIstanbul([]byte("{ this is not json"), t.TempDir())
	if err == nil {
		t.Fatal("ParseIstanbul() error = nil, want a JSON syntax error")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("ParseIstanbul() error = %v, want it to describe the JSON syntax failure", err)
	}
}

func TestParseIstanbul_FallsBackToMapKeyAndResolvesRelativePaths(t *testing.T) {
	root := t.TempDir()
	absKey := filepath.Join(root, "src", "a.ts")
	relKey := filepath.Join(root, "src", "c.ts")
	loc := istanbulLocation{Start: istanbulPosition{Line: 1}, End: istanbulPosition{Line: 1, Column: 1}}
	body := marshalIstanbul(t, map[string]istanbulFile{
		// path is empty: the map key must be used as the file's absolute path.
		absKey: {StatementMap: map[string]istanbulLocation{"0": loc}, S: map[string]int{"0": 1}},
		// path is relative: it must be joined with the directory of the map key.
		relKey: {Path: "b.ts", StatementMap: map[string]istanbulLocation{"0": loc}, S: map[string]int{"0": 1}},
	})

	files, warnings, err := ParseIstanbul([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseIstanbul() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	got := make([]string, 0, len(files))
	for _, f := range files {
		got = append(got, f.Path)
	}
	want := []string{"src/a.ts", "src/b.ts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v (empty path uses the key, relative path joins the key's dir)", got, want)
	}
}

// marshalIstanbul builds a coverage-final.json body from typed entries so the
// tests never hand-write JSON quoting.
func marshalIstanbul(t *testing.T, raw map[string]istanbulFile) string {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// --- lineCoverage ---

func TestLineCoverage_UsesExplicitLineMapWhenPresent(t *testing.T) {
	f := istanbulFile{
		L: map[string]int{"1": 5, "2": 0, "not-a-line": 9},
		// Statements must be ignored entirely when the l map exists.
		StatementMap: map[string]istanbulLocation{"0": {Start: istanbulPosition{Line: 9}}},
		S:            map[string]int{"0": 1},
	}

	got := lineCoverage(f)
	want := map[int]int{1: 5, 2: 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lineCoverage() = %v, want %v (l is authoritative; non-numeric keys are skipped)", got, want)
	}
}

func TestLineCoverage_SkipsNonPositiveLinesAndKeepsHighestHitCount(t *testing.T) {
	f := istanbulFile{
		StatementMap: map[string]istanbulLocation{
			"0": {Start: istanbulPosition{Line: 0}},
			"1": {Start: istanbulPosition{Line: 5}},
			"2": {Start: istanbulPosition{Line: 5}},
		},
		S: map[string]int{"0": 7, "1": 3, "2": 1},
	}

	got := lineCoverage(f)
	want := map[int]int{5: 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lineCoverage() = %v, want %v (line 0 dropped; the highest count on line 5 wins)", got, want)
	}
}

// --- functionCoverage ---

func TestFunctionCoverage_ReturnsNilWithoutFunctions(t *testing.T) {
	if got := functionCoverage(istanbulFile{}); got != nil {
		t.Errorf("functionCoverage() = %+v, want nil when the file declares no functions", got)
	}
}

func TestFunctionCoverage_FallsBackToDeclThenLine(t *testing.T) {
	f := istanbulFile{
		FnMap: map[string]istanbulFunction{
			// loc is empty: the decl location must be used.
			"0": {Name: "declOnly", Decl: istanbulLocation{Start: istanbulPosition{Line: 5}}, Line: 99},
			// loc and decl are both empty: fn.line is the last resort.
			"1": {Name: "lineOnly", Line: 42},
			// blank name: the anonymous fallback must name it by position.
			"2": {Name: "   ", Loc: istanbulLocation{Start: istanbulPosition{Line: 8}}},
		},
		F: map[string]int{"0": 1, "1": 0, "2": 0},
	}

	byName := map[string]model.FuncCoverage{}
	for _, fn := range functionCoverage(f) {
		byName[fn.Name] = fn
	}

	decl, ok := byName["declOnly"]
	if !ok {
		t.Fatalf("functions = %v, want declOnly present", byName)
	}
	if decl.Line != 5 {
		t.Errorf("declOnly.Line = %d, want 5 taken from decl when loc is empty", decl.Line)
	}
	if !decl.Covered || decl.Hits != 1 {
		t.Errorf("declOnly = %+v, want covered with 1 hit", decl)
	}

	line, ok := byName["lineOnly"]
	if !ok {
		t.Fatalf("functions = %v, want lineOnly present", byName)
	}
	if line.Line != 42 {
		t.Errorf("lineOnly.Line = %d, want 42 from fn.line when both loc and decl are empty", line.Line)
	}

	anon, ok := byName["(anonymous_3)"]
	if !ok {
		t.Fatalf("functions = %v, want the blank name at index 3 to become (anonymous_3)", byName)
	}
	if anon.Line != 8 {
		t.Errorf("(anonymous_3).Line = %d, want 8 from the loc start when decl is empty", anon.Line)
	}
}

func TestFunctionCoverage_AssignsInnermostOwnerAndSkipsTopLevelStatements(t *testing.T) {
	f := istanbulFile{
		StatementMap: map[string]istanbulLocation{
			// Inside both outer (lines 1-10) and inner (lines 2-4).
			"0": {Start: istanbulPosition{Line: 3, Column: 1}, End: istanbulPosition{Line: 3, Column: 2}},
			// Owned by no function, so it must not appear anywhere.
			"1": {Start: istanbulPosition{Line: 50, Column: 1}, End: istanbulPosition{Line: 50, Column: 2}},
		},
		FnMap: map[string]istanbulFunction{
			"0": {Name: "outer", Decl: istanbulLocation{Start: istanbulPosition{Line: 1}},
				Loc: istanbulLocation{Start: istanbulPosition{Line: 1}, End: istanbulPosition{Line: 10}}, Line: 1},
			"1": {Name: "inner", Decl: istanbulLocation{Start: istanbulPosition{Line: 2}},
				Loc: istanbulLocation{Start: istanbulPosition{Line: 2}, End: istanbulPosition{Line: 4}}, Line: 2},
		},
		S: map[string]int{"0": 2, "1": 9},
		F: map[string]int{"0": 1, "1": 1},
	}

	byName := map[string]model.FuncCoverage{}
	for _, fn := range functionCoverage(f) {
		byName[fn.Name] = fn
	}

	inner, ok := byName["inner"]
	if !ok {
		t.Fatalf("functions = %v, want inner present", byName)
	}
	if want := (model.Metric{Covered: 1, Total: 1}); inner.Lines != want {
		t.Errorf("inner.Lines = %+v, want %+v so the innermost owner claims the statement", inner.Lines, want)
	}
	outer, ok := byName["outer"]
	if !ok {
		t.Fatalf("functions = %v, want outer present", byName)
	}
	if outer.Lines.Total != 0 {
		t.Errorf("outer.Lines = %+v, want 0/0 so the statement is not double-counted", outer.Lines)
	}
}

func TestFunctionCoverage_SortsSameLineFunctionsByName(t *testing.T) {
	f := istanbulFile{
		FnMap: map[string]istanbulFunction{
			"0": {Name: "zeta", Decl: istanbulLocation{Start: istanbulPosition{Line: 7}},
				Loc: istanbulLocation{Start: istanbulPosition{Line: 7}, End: istanbulPosition{Line: 8}}},
			"1": {Name: "alpha", Decl: istanbulLocation{Start: istanbulPosition{Line: 7}},
				Loc: istanbulLocation{Start: istanbulPosition{Line: 7}, End: istanbulPosition{Line: 8}}},
		},
		F: map[string]int{},
	}

	got := functionCoverage(f)
	if len(got) != 2 {
		t.Fatalf("functionCoverage() = %+v, want two entries", got)
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("order = [%s %s], want [alpha zeta] so equal-line functions sort by name", got[0].Name, got[1].Name)
	}
}

// --- branchMetric ---

func TestBranchMetric_CountsDeclaredButUninstrumentedBranches(t *testing.T) {
	f := istanbulFile{
		BranchMap: map[string]istanbulBranch{"0": {}, "1": {}},
		// Branch "0" has no b entry at all; it must still count in the total.
		B: map[string][]int{"1": {3, 0}},
	}

	got, ok := branchMetric(f)
	if !ok {
		t.Fatal("branchMetric() ok = false, want true when branches are declared")
	}
	want := model.Metric{Covered: 1, Total: 3}
	if got != want {
		t.Errorf("branchMetric() = %+v, want %+v (a branch with no b entry still counts)", got, want)
	}
}

func TestBranchMetric_ReportsNoBranches(t *testing.T) {
	if got, ok := branchMetric(istanbulFile{}); ok {
		t.Errorf("branchMetric() = (%+v, true), want ok=false for a file with no branch map", got)
	}
}

// --- ParseLcov ---

func TestParseLcov_SkipsRecordsOutsideRootAndUnsupportedTypes(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "..", "elsewhere", "a.ts")
	css := filepath.Join(root, "styles.css")
	body := "SF:" + outside + "\nDA:1,1\nend_of_record\n" +
		"SF:" + css + "\nDA:1,1\nend_of_record\n" +
		"end_of_record\n" // no record is open: must be ignored, not panic.

	files, warnings, err := ParseLcov([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseLcov() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files = %+v, want none", files)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one summary", warnings)
	}
	want := "ignored 2 lcov records outside the scan root or of an unsupported file type"
	if warnings[0] != want {
		t.Errorf("warnings[0] = %q, want %q", warnings[0], want)
	}
}

func TestParseLcov_IgnoresMalformedDirectives(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "a.ts")
	body := "SF:" + src + "\n" +
		"DA:1\n" + // fewer than two fields
		"DA:x,1\n" + // non-numeric line
		"DA:1,y\n" + // non-numeric hit count
		"FN:1\n" + // no comma at all
		"FN:x,alpha\n" + // non-numeric declaration line
		"FNDA:1\n" + // no comma at all
		"FNDA:x,alpha\n" + // non-numeric hit count
		"BRDA:1,0,0\n" + // fewer than four fields
		"DA:1,2\n" +
		"FN:1,alpha\n" +
		"FNDA:1,alpha\n" +
		"BRDA:1,0,0,1\n" +
		"end_of_record\n"

	files, warnings, err := ParseLcov([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseLcov() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none for malformed directives (they are silently dropped)", warnings)
	}
	if len(files) != 1 {
		t.Fatalf("files = %+v, want the one well-formed record", files)
	}
	got := files[0]
	if want := (model.Metric{Covered: 1, Total: 1}); got.Lines != want {
		t.Errorf("Lines = %+v, want %+v", got.Lines, want)
	}
	if want := (model.Metric{Covered: 1, Total: 1}); got.Functions != want {
		t.Errorf("Functions = %+v, want %+v", got.Functions, want)
	}
	if got.Branches == nil || *got.Branches != (model.Metric{Covered: 1, Total: 1}) {
		t.Errorf("Branches = %+v, want 1/1", got.Branches)
	}
}

func TestParseLcov_MatchesRepeatedFunctionNamesToFirstUnmatched(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "a.ts")
	body := "SF:" + src + "\n" +
		"FN:1,dup\n" +
		"FN:2,dup\n" +
		"FNDA:5,dup\n" +
		"FNDA:3,dup\n" +
		"DA:1,1\n" +
		"end_of_record\n"

	files, _, err := ParseLcov([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseLcov() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %+v, want one record", files)
	}
	fns := files[0].FunctionList
	if len(fns) != 2 {
		t.Fatalf("functions = %+v, want two entries for the repeated name", fns)
	}
	if fns[0].Hits != 5 || fns[1].Hits != 3 {
		t.Errorf("hits = [%d %d], want [5 3] so each FNDA binds to the first still-unmatched name",
			fns[0].Hits, fns[1].Hits)
	}
}

func TestParseLcov_TreatsDashAndZeroBranchesAsUntaken(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "a.ts")
	body := "SF:" + src + "\n" +
		"BRDA:1,0,0,1\n" +
		"BRDA:1,0,1,-\n" +
		"BRDA:1,0,2,0\n" +
		"DA:1,1\n" +
		"end_of_record\n"

	files, _, err := ParseLcov([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseLcov() error = %v", err)
	}
	if files[0].Branches == nil {
		t.Fatal("Branches = nil, want a metric")
	}
	if want := (model.Metric{Covered: 1, Total: 3}); *files[0].Branches != want {
		t.Errorf("Branches = %+v, want %+v ('-' and '0' are untaken yet still counted)", *files[0].Branches, want)
	}
}

func TestParseLcov_HandlesStrayEndOfRecordAndConsecutiveSF(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "src", "a.ts")
	second := filepath.Join(root, "src", "b.ts")
	body := "end_of_record\n" + // nothing open yet
		"SF:" + first + "\n" +
		"SF:" + second + "\n" + // closes the empty first record
		"DA:1,4\n" +
		"end_of_record\n"

	files, warnings, err := ParseLcov([]byte(body), root)
	if err != nil {
		t.Fatalf("ParseLcov() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if len(files) != 2 {
		t.Fatalf("files = %+v, want the empty first record and the populated second", files)
	}
	if files[0].Path != "src/a.ts" || files[0].Lines.Total != 0 {
		t.Errorf("files[0] = %+v, want the still-empty src/a.ts record", files[0])
	}
	if files[1].Path != "src/b.ts" || files[1].Lines != (model.Metric{Covered: 1, Total: 1}) {
		t.Errorf("files[1] = %+v, want src/b.ts with 1/1 lines", files[1])
	}
}

func TestParseLcov_ReturnsScannerErrorForOversizedLine(t *testing.T) {
	// The scanner's token cap is 8 MiB; a single longer line makes it fail.
	huge := strings.Repeat("x", 9<<20)

	files, warnings, err := ParseLcov([]byte(huge), t.TempDir())
	if err == nil {
		t.Fatal("ParseLcov() error = nil, want the scanner's token-too-long error")
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Errorf("ParseLcov() error = %v, want bufio.ErrTooLong", err)
	}
	if files != nil {
		t.Errorf("files = %+v, want nil when scanning aborted", files)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

// --- shared helpers ---

func TestTruncate_CapsLongOutput(t *testing.T) {
	if got, want := truncate("abcdefghij", 4), "abcd\n... (output truncated)"; got != want {
		t.Errorf("truncate() = %q, want %q", got, want)
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Errorf("truncate() = %q, want the input unchanged when it fits", got)
	}
}

func TestExitCode_ReturnsMinusOneForNonExitError(t *testing.T) {
	if got := exitCode(errors.New("not an exec failure")); got != -1 {
		t.Errorf("exitCode() = %d, want -1 for a non-*exec.ExitError", got)
	}
}
