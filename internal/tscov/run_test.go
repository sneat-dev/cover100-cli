package tscov

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/detect"
	"github.com/sneat-dev/cover100-cli/internal/model"
)

// fakeRunner installs a stub test runner at <pkgDir>/node_modules/.bin/<runner>
// that writes reportBody as the coverage artefact the real runner would
// produce. It lets the tests drive Run's success, failure and missing-report
// branches without npm, jest or vitest being installed.
func fakeRunner(t *testing.T, pkgDir, runner, reportName, reportBody string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub runner is a POSIX shell script")
	}

	binDir := filepath.Join(pkgDir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	write := ""
	if reportName != "" {
		write = fmt.Sprintf("mkdir -p \"$out\"\ncat > \"$out/%s\" <<'REPORT'\n%s\nREPORT\n", reportName, reportBody)
	}
	script := fmt.Sprintf(`#!/bin/sh
out=""
for arg in "$@"; do
  case "$arg" in
    --coverage.reportsDirectory=*) out="${arg#--coverage.reportsDirectory=}" ;;
    --coverageDirectory=*) out="${arg#--coverageDirectory=}" ;;
  esac
done
%s
echo "stub %s ran with: $*"
exit %d
`, write, runner, exitCode)

	if err := os.WriteFile(filepath.Join(binDir, runner), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// istanbulBody is a minimal coverage-final.json for one TypeScript file with
// two statements (one covered) and one covered function.
func istanbulBody(absPath string) string {
	return fmt.Sprintf(`{"%s":{"path":"%s","statementMap":{"0":{"start":{"line":1,"column":0},"end":{"line":1,"column":9}},"1":{"start":{"line":3,"column":0},"end":{"line":3,"column":9}}},"fnMap":{"0":{"name":"load","decl":{"start":{"line":1,"column":0},"end":{"line":1,"column":4}},"loc":{"start":{"line":1,"column":0},"end":{"line":3,"column":10}},"line":1}},"branchMap":{},"s":{"0":2,"1":0},"f":{"0":2},"b":{}}}`,
		absPath, absPath)
}

func packageFixture(t *testing.T) (root, pkgDir string) {
	t.Helper()
	root = t.TempDir()
	pkgDir = filepath.Join(root, "web")
	if err := os.MkdirAll(filepath.Join(pkgDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"),
		[]byte(`{"name":"web","scripts":{"test":"vitest run"},"devDependencies":{"vitest":"^2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "src", "a.ts"), []byte("export const a = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, pkgDir
}

func projectFor(root, pkgDir, runner string) detect.Project {
	rel, _ := filepath.Rel(root, pkgDir)
	return detect.Project{
		Dir: pkgDir, Rel: filepath.ToSlash(rel), Kind: detect.KindNode,
		PackageName: "web", TestScript: runner + " run",
		HasTestScript: true, Runner: runner, HasNodeModules: true,
	}
}

func TestRun_ParsesIstanbulReport(t *testing.T) {
	root, pkgDir := packageFixture(t)
	fakeRunner(t, pkgDir, runnerVitest, "coverage-final.json",
		istanbulBody(filepath.Join(pkgDir, "src", "a.ts")), 0)

	res := Run(context.Background(), projectFor(root, pkgDir, runnerVitest), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err != nil {
		t.Fatalf("Run() error = %v (warnings: %v)", res.Err, res.Warnings)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want one file", res.Files)
	}
	got := res.Files[0]
	if got.Path != "web/src/a.ts" {
		t.Errorf("Path = %q, want web/src/a.ts", got.Path)
	}
	if want := (model.Metric{Covered: 1, Total: 2}); got.Lines != want {
		t.Errorf("Lines = %+v, want %+v", got.Lines, want)
	}
	if want := (model.Metric{Covered: 1, Total: 1}); got.Functions != want {
		t.Errorf("Functions = %+v, want %+v", got.Functions, want)
	}
	if res.Runner != runnerVitest {
		t.Errorf("Runner = %q, want vitest", res.Runner)
	}
	if !strings.Contains(res.Command, "vitest") {
		t.Errorf("Command = %q, want it to name the runner", res.Command)
	}
}

func TestRun_ParsesJestReport(t *testing.T) {
	root, pkgDir := packageFixture(t)
	fakeRunner(t, pkgDir, runnerJest, "coverage-final.json",
		istanbulBody(filepath.Join(pkgDir, "src", "a.ts")), 0)

	res := Run(context.Background(), projectFor(root, pkgDir, runnerJest), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err != nil {
		t.Fatalf("Run() error = %v (warnings: %v)", res.Err, res.Warnings)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want one file", res.Files)
	}
}

func TestRun_FallsBackToLcov(t *testing.T) {
	root, pkgDir := packageFixture(t)
	lcov := "SF:" + filepath.Join(pkgDir, "src", "a.ts") + "\nDA:1,1\nDA:2,0\nFN:1,load\nFNDA:1,load\nend_of_record"
	fakeRunner(t, pkgDir, runnerVitest, "lcov.info", lcov, 0)

	res := Run(context.Background(), projectFor(root, pkgDir, runnerVitest), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err != nil {
		t.Fatalf("Run() error = %v (warnings: %v)", res.Err, res.Warnings)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want the lcov fallback to be parsed", res.Files)
	}
	if want := (model.Metric{Covered: 1, Total: 2}); res.Files[0].Lines != want {
		t.Errorf("Lines = %+v, want %+v", res.Files[0].Lines, want)
	}
}

func TestRun_WarnsWhenNoReportIsProduced(t *testing.T) {
	root, pkgDir := packageFixture(t)
	fakeRunner(t, pkgDir, runnerVitest, "", "", 0)

	res := Run(context.Background(), projectFor(root, pkgDir, runnerVitest), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if len(res.Files) != 0 {
		t.Errorf("Files = %+v, want none", res.Files)
	}
	if !hasWarning(res.Warnings, "no coverage report was produced") {
		t.Errorf("Warnings = %v, want one explaining the missing report", res.Warnings)
	}
}

func TestRun_StillParsesReportWhenTestsFail(t *testing.T) {
	root, pkgDir := packageFixture(t)
	fakeRunner(t, pkgDir, runnerVitest, "coverage-final.json",
		istanbulBody(filepath.Join(pkgDir, "src", "a.ts")), 1)

	res := Run(context.Background(), projectFor(root, pkgDir, runnerVitest), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err == nil {
		t.Fatal("Run() must report the failing test command")
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", res.ExitCode)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want the written report to be parsed anyway", res.Files)
	}
	if !hasWarning(res.Warnings, "test failures") {
		t.Errorf("Warnings = %v, want one naming the test failure", res.Warnings)
	}
}

func TestRun_DefaultsToJestWhenNoRunnerIsDetected(t *testing.T) {
	root, pkgDir := packageFixture(t)
	// The project claims no runner, so Run must fall back to jest and say so.
	fakeRunner(t, pkgDir, runnerJest, "coverage-final.json",
		istanbulBody(filepath.Join(pkgDir, "src", "a.ts")), 0)

	project := projectFor(root, pkgDir, "")
	res := Run(context.Background(), project, Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Runner != runnerJest {
		t.Errorf("Runner = %q, want jest", res.Runner)
	}
	if !hasWarning(res.Warnings, "no test runner dependency") {
		t.Errorf("Warnings = %v, want one naming the assumed runner", res.Warnings)
	}
}

func hasWarning(warnings []string, substring string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substring) {
			return true
		}
	}
	return false
}
