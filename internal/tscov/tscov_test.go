package tscov

import (
	"fmt"
	"path/filepath"
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
