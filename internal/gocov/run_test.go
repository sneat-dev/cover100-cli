package gocov

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/detect"
)

// fakeGo installs a stub `go` executable on PATH and returns nothing. The stub
// writes the given profile body to the path passed via -coverprofile and exits
// with exitCode, which lets the tests drive every branch of Run without
// depending on a real toolchain or on real test outcomes.
func fakeGo(t *testing.T, profileBody string, exitCode int) {
	t.Helper()
	fakeGoWith(t, profileBody, exitCode, true)
}

// fakeGoWith is fakeGo with control over whether the stub creates the profile
// file at all, which is how "go test never wrote a profile" is exercised.
func fakeGoWith(t *testing.T, profileBody string, exitCode int, writeProfile bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub runner is a POSIX shell script")
	}

	binDir := t.TempDir()
	script := `#!/bin/sh
out=""
for arg in "$@"; do
  case "$arg" in
    -coverprofile=*) out="${arg#-coverprofile=}" ;;
  esac
done
if [ -n "$out" ] && [ "` + boolFlag(writeProfile) + `" = "yes" ]; then
  cat > "$out" <<'PROFILE'
` + profileBody + `
PROFILE
fi
echo "stub go ran"
exit ` + itoa(exitCode) + `
`
	path := filepath.Join(binDir, "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// boolFlag renders a Go bool as a shell test string.
func boolFlag(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return string(rune('0' + n))
}

// moduleFixture creates a directory that looks like a Go module.
func moduleFixture(t *testing.T) (root, moduleDir string) {
	t.Helper()
	root = t.TempDir()
	moduleDir = filepath.Join(root, "services", "api")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, moduleDir
}

func projectFor(root, moduleDir string) detect.Project {
	rel, _ := filepath.Rel(root, moduleDir)
	return detect.Project{
		Dir: moduleDir, Rel: filepath.ToSlash(rel),
		Kind: detect.KindGo, ModulePath: "example.com/fixture",
	}
}

func TestRun_ParsesProfileAndReportsRelativePaths(t *testing.T) {
	fakeGo(t, `mode: set
example.com/fixture/pkg/calc.go:1.1,2.1 1 1
example.com/fixture/pkg/calc.go:5.1,6.1 1 0
example.com/fixture/pkg/calc_test.go:1.1,2.1 1 1`, 0)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err != nil {
		t.Fatalf("Run() error = %v (warnings: %v)", res.Err, res.Warnings)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want one non-test file", res.Files)
	}
	got := res.Files[0]
	if got.Path != "services/api/pkg/calc.go" {
		t.Errorf("Path = %q, want services/api/pkg/calc.go", got.Path)
	}
	// The first block spans lines 1-2, both covered; the second spans 5-6,
	// both uncovered.
	if want := 2; got.Lines.Covered != want {
		t.Errorf("Lines.Covered = %d, want %d", got.Lines.Covered, want)
	}
	if want := 4; got.Lines.Total != want {
		t.Errorf("Lines.Total = %d, want %d (uncovered lines must count)", got.Lines.Total, want)
	}
	if got.FunctionsAvailable {
		t.Error("FunctionsAvailable = true, want false: Go profiles carry no function data")
	}
	if got.Functions.Total != 0 {
		t.Errorf("Functions = %+v, want 0/0", got.Functions)
	}
	if res.Command == "" {
		t.Error("Command must record what was run")
	}
}

func TestRun_StillParsesPartialProfileWhenTestsFail(t *testing.T) {
	fakeGo(t, `mode: set
example.com/fixture/pkg/calc.go:1.1,2.1 1 1`, 1)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err == nil {
		t.Fatal("Run() must report the failing test command")
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", res.ExitCode)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want the partial profile to be parsed anyway", res.Files)
	}
	if !hasWarning(res.Warnings, "test failures") {
		t.Errorf("Warnings = %v, want one naming the test failure", res.Warnings)
	}
}

func TestRun_WarnsWhenNoProfileFileExists(t *testing.T) {
	fakeGoWith(t, "", 0, false)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if len(res.Files) != 0 {
		t.Errorf("Files = %+v, want none", res.Files)
	}
	if !hasWarning(res.Warnings, "no coverage profile") {
		t.Errorf("Warnings = %v, want one explaining that no profile was written", res.Warnings)
	}
}

func TestRun_WarnsWhenTheProfileIsEmpty(t *testing.T) {
	fakeGo(t, "", 0)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if !hasWarning(res.Warnings, "empty") {
		t.Errorf("Warnings = %v, want one explaining that the profile is empty", res.Warnings)
	}
}

func TestRun_IgnoresEntriesOutsideTheModule(t *testing.T) {
	fakeGo(t, `mode: set
example.com/fixture/pkg/calc.go:1.1,2.1 1 1
example.com/other/x.go:1.1,2.1 1 1`, 0)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want only the module's own file", res.Files)
	}
	if !hasWarning(res.Warnings, "outside the module directory") {
		t.Errorf("Warnings = %v, want one naming the skipped entries", res.Warnings)
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
