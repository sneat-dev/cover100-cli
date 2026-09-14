package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sneat-dev/cover100-cli/internal/model"
	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

// testAssets mirrors the embedded layout: NewRootCmd receives a filesystem
// rooted at the repository, and the viewer lives under public/.
func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"public/index.html": &fstest.MapFile{Data: []byte(
			`<!DOCTYPE html><html><head><link rel="stylesheet" href="styles.css"></head>` +
				`<body><script src="app.js"></script></body></html>`)},
		"public/styles.css": &fstest.MapFile{Data: []byte("/* css */")},
		"public/app.js":     &fstest.MapFile{Data: []byte("// app")},
	}
}

// executeWith runs the root command with args, directing its output to out and
// errOut.
func executeWith(t *testing.T, out, errOut io.Writer, args ...string) error {
	t.Helper()
	root, _ := NewRootCmd(testAssets())
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errOut)
	return root.Execute()
}

// execute runs the root command with its output discarded.
func execute(t *testing.T, args ...string) error {
	t.Helper()
	return executeWith(t, io.Discard, io.Discard, args...)
}

// exitCodeOf returns the exit code carried by err.
func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) {
		t.Fatalf("error %v does not carry an exit code", err)
	}
	return coded.ExitCode()
}

// writeGoFixture creates a tiny Go module with one covered function and one
// deliberately uncovered function, so the report must show partial coverage
// rather than a suspicious 100%.
func writeGoFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/fixture\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(dir, "pkg", "calc.go"), `package pkg

// Covered is exercised by the fixture test.
func Covered(a, b int) int {
	return a + b
}

// Uncovered is deliberately never called by a test.
func Uncovered(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`)
	writeTestFile(t, filepath.Join(dir, "pkg", "calc_test.go"), `package pkg

import "testing"

func TestCovered(t *testing.T) {
	if Covered(2, 3) != 5 {
		t.Fatal("bad sum")
	}
}
`)
	return dir
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireGoToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is required to collect Go coverage")
	}
}

func TestRun_EndToEndProducesReportAndStandaloneViewer(t *testing.T) {
	requireGoToolchain(t)

	fixture := writeGoFixture(t)
	outDir := t.TempDir()
	reportPath := filepath.Join(outDir, "coverage.json")

	if err := execute(t, fixture, "--no-serve", "--no-open", "--out", reportPath); err != nil {
		t.Fatalf("cover100 exited with error: %v", err)
	}

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report was not written: %v", err)
	}
	var report model.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}

	if report.Root != fixture {
		t.Errorf("root = %q, want %q", report.Root, fixture)
	}
	if len(report.Metrics) != 5 {
		t.Errorf("metrics = %v, want all five", report.Metrics)
	}
	if len(report.Languages) != 1 || report.Languages[0] != model.LangGo {
		t.Errorf("languages = %v, want [go]", report.Languages)
	}
	if report.Tool == "" {
		t.Error("tool identity must be recorded in the report")
	}

	// The fixture has an uncovered function, so the run must not report 100%.
	// A regression here means uncovered lines vanished from the denominator.
	lines := report.Tree.Lines
	if lines.Total == 0 {
		t.Fatal("no lines measured: the coverage profile was not parsed")
	}
	if lines.Covered >= lines.Total {
		t.Errorf("lines = %d/%d, want partial coverage (the fixture has an uncovered function)",
			lines.Covered, lines.Total)
	}

	// The standalone viewer is what --file opens, and it must be
	// self-contained.
	viewerPath := filepath.Join(outDir, standaloneName)
	viewer, err := os.ReadFile(viewerPath)
	if err != nil {
		t.Fatalf("standalone viewer was not written: %v", err)
	}
	if len(viewer) <= len(raw) {
		t.Error("standalone viewer should embed the report plus the page assets")
	}
	for _, want := range []string{"<style>", "__COVER100_DATA__", "// app"} {
		if !strings.Contains(string(viewer), want) {
			t.Errorf("standalone viewer missing %q", want)
		}
	}

	// Collection artefacts must be cleaned up by default, while the report and
	// the viewer — which live in the same directory — must survive.
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "coverage.json" || name == standaloneName {
			continue
		}
		t.Errorf("unexpected leftover artefact %q: cleanup must remove intermediates but keep the report", name)
	}
}

func TestRun_KeepRetainsIntermediates(t *testing.T) {
	requireGoToolchain(t)

	fixture := writeGoFixture(t)
	outDir := t.TempDir()

	if err := execute(t, fixture, "--no-serve", "--no-open", "--keep",
		"--out", filepath.Join(outDir, "coverage.json")); err != nil {
		t.Fatalf("cover100 exited with error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "go-0.out")); err != nil {
		t.Errorf("--keep must retain the coverage profile: %v", err)
	}
}

func TestRun_JSONFormatEmitsOnlyTheSummary(t *testing.T) {
	requireGoToolchain(t)

	fixture := writeGoFixture(t)
	outDir := t.TempDir()
	reportPath := filepath.Join(outDir, "coverage.json")

	var stdout bytes.Buffer
	if err := executeWith(t, &stdout, io.Discard,
		fixture, "--no-serve", "--no-open", "--format", "json", "--out", reportPath); err != nil {
		t.Fatalf("cover100 exited with error: %v", err)
	}

	var sum summary
	if err := json.Unmarshal(stdout.Bytes(), &sum); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v (output: %q)", err, stdout.String())
	}
	if sum.Report != reportPath {
		t.Errorf("summary report = %q, want %q", sum.Report, reportPath)
	}
	if len(sum.Metrics) != 5 {
		t.Errorf("summary metrics = %+v, want all five", sum.Metrics)
	}

	var lines metricSummary
	for _, m := range sum.Metrics {
		if m.Metric == "lines" {
			lines = m
		}
	}
	if lines.Total == 0 || lines.Covered >= lines.Total {
		t.Errorf("summary lines = %+v, want partial coverage", lines)
	}
}

func TestRun_MissingPathExitsNotFound(t *testing.T) {
	err := execute(t, filepath.Join(t.TempDir(), "nope"), "--no-serve", "--no-open")
	if err == nil {
		t.Fatal("expected an error for a missing scan path")
	}
	if got := exitCodeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestRun_NoProjectsExitsNotFound(t *testing.T) {
	err := execute(t, t.TempDir(), "--no-serve", "--no-open")
	if err == nil {
		t.Fatal("expected an error when nothing is detected")
	}
	if got := exitCodeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestRun_InvalidFlagExitsInvalidArgs(t *testing.T) {
	err := execute(t, t.TempDir(), "--no-serve", "--metric", "statements")
	if err == nil {
		t.Fatal("expected an error for an invalid --metric")
	}
	if got := exitCodeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestRun_LanguageRestrictionExcludesGoProjects(t *testing.T) {
	requireGoToolchain(t)

	fixture := writeGoFixture(t)
	outDir := t.TempDir()
	reportPath := filepath.Join(outDir, "coverage.json")

	// The fixture is Go-only, so restricting to TypeScript must find nothing
	// and report that as exit code 3 rather than an empty report.
	err := execute(t, fixture, "--no-serve", "--no-open", "--lang", "ts", "--out", reportPath)
	if err == nil {
		t.Fatal("expected an error when --lang excludes every detected project")
	}
	if got := exitCodeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
	if _, statErr := os.Stat(reportPath); statErr == nil {
		t.Error("no report should be written when nothing was collected")
	}
}
