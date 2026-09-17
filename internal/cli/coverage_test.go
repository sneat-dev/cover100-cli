package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/cliinstall"

	"github.com/sneat-dev/cover100-cli/internal/detect"
	"github.com/sneat-dev/cover100-cli/internal/gocov"
	"github.com/sneat-dev/cover100-cli/internal/model"
	"github.com/sneat-dev/cover100-cli/internal/serve"
	"github.com/sneat-dev/cover100-cli/internal/tscov"
	"github.com/sneat-dev/cover100-cli/internal/ui"
	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

// --- shared helpers ---

// captureStderr redirects os.Stderr for the duration of fn, so a test can
// assert on what the process really writes rather than on a buffer it injected.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// captureStdout redirects os.Stdout for the duration of fn.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// failingWriter refuses every write, for exercising write-failure branches.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write refused") }

// quietCommand returns a command whose output goes nowhere.
func quietCommand() *cobra.Command {
	cmd := &cobra.Command{Use: binaryName, SilenceUsage: true, SilenceErrors: true}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// collectOptsFor returns the options a plain
// `cover100 <fixture> --no-serve --no-open --out <out>` run would have.
func collectOptsFor(out string) *collectOptions {
	return &collectOptions{
		out:     out,
		open:    false,
		noOpen:  true,
		lang:    "all",
		metric:  "lines",
		mode:    "percent",
		port:    0,
		timeout: time.Minute,
		format:  "text",
		noServe: true,
	}
}

func requirePOSIXStubs(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub tools are POSIX shell scripts")
	}
}

// stubCommand writes an executable POSIX shell script called name into a temp
// dir and returns that directory, so a test can put a fake tool on PATH.
func stubCommand(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeStubAt(t, filepath.Join(dir, name), body)
	return dir
}

func writeStubAt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func hasSubstring(values []string, substring string) bool {
	for _, v := range values {
		if strings.Contains(v, substring) {
			return true
		}
	}
	return false
}

func exitCodeOfError(t *testing.T, err error) int {
	t.Helper()
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) {
		t.Fatalf("error %v does not carry an exit code", err)
	}
	return coded.ExitCode()
}

// --- root.go ---

func TestRun_VersionFlagSucceeds(t *testing.T) {
	out := captureStdout(t, func() {
		if err := Run([]string{binaryName, "--version"}, testAssets()); err != nil {
			t.Errorf("Run(--version) = %v, want nil", err)
		}
	})
	if got := strings.TrimSpace(out); got != buildInfo.Short() {
		t.Errorf("--version printed %q, want %q", got, buildInfo.Short())
	}
}

func TestRun_InvalidFlagCarriesInvalidArgsAndIsNotPrintedByTheFramework(t *testing.T) {
	// silentErrorHandler exists so the framework does not report the error a
	// second time; Fatal owns reporting. Assert both halves of that contract.
	var err error
	stderr := captureStderr(t, func() {
		err = Run([]string{binaryName, "--metric", "statements"}, testAssets())
	})

	if err == nil {
		t.Fatal("Run() = nil, want an error for an invalid --metric")
	}
	if got := exitCodeOfError(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", got, exitcode.InvalidArgs)
	}
	if strings.Contains(stderr, err.Error()) {
		t.Errorf("the framework printed the error itself (%q); Fatal would report it twice", stderr)
	}
}

func TestExecuteWithPanicRecovery_ConvertsAPanicIntoExit10(t *testing.T) {
	root := &cobra.Command{
		Use:           "boom",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(*cobra.Command, []string) error { panic("deliberate test panic") },
	}

	var err error
	stderr := captureStderr(t, func() { err = executeWithPanicRecovery(root) })

	if err == nil {
		t.Fatal("executeWithPanicRecovery() = nil, want an error")
	}
	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
	if !strings.Contains(err.Error(), "panic recovered") || !strings.Contains(err.Error(), "deliberate test panic") {
		t.Errorf("error = %q, want it to name the recovered panic", err)
	}
	// The stack must reach the user, or a crash is undebuggable.
	if !strings.Contains(stderr, "deliberate test panic") || !strings.Contains(stderr, "goroutine") {
		t.Errorf("stderr = %q, want the panic value and a stack trace", stderr)
	}
}

func TestExecuteWithPanicRecovery_PassesThroughNormalErrors(t *testing.T) {
	want := exitcode.NotFoundError("nothing to scan")
	root := &cobra.Command{
		Use:           "quiet",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(*cobra.Command, []string) error { return want },
	}

	var err error
	_ = captureStderr(t, func() { err = executeWithPanicRecovery(root) })

	if !errors.Is(err, want) {
		t.Errorf("error = %v, want the command's own error %v", err, want)
	}
}

func TestFatal(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{"nil is a no-op", nil, -1, ""},
		{"a coded error keeps its code", exitcode.NotFoundError("nothing to scan"), exitcode.NotFound, "nothing to scan"},
		{"an interrupt keeps 130", exitcode.New(130, "interrupted"), 130, "interrupted"},
		{"an uncoded error exits 1", errors.New("plain failure"), 1, "plain failure"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := osExit
			t.Cleanup(func() { osExit = original })

			gotCode := -1
			osExit = func(code int) { gotCode = code }

			stderr := captureStderr(t, func() { Fatal(tc.err) })

			if gotCode != tc.wantCode {
				t.Errorf("exit code = %d, want %d", gotCode, tc.wantCode)
			}
			if tc.wantMsg == "" {
				if stderr != "" {
					t.Errorf("stderr = %q, want nothing for a nil error", stderr)
				}
				return
			}
			if !strings.Contains(stderr, tc.wantMsg) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantMsg)
			}
		})
	}
}

func TestNewRootCmd_HasTheDocumentedSurface(t *testing.T) {
	root, fangOpts := NewRootCmd(testAssets())

	if root.Use != binaryName+" [path]" {
		t.Errorf("Use = %q, want %q", root.Use, binaryName+" [path]")
	}
	if !root.SilenceUsage || !root.SilenceErrors {
		t.Error("the root command must own its error reporting")
	}
	if root.Version != buildInfo.Short() {
		t.Errorf("Version = %q, want %q so --version and the version subcommand agree", root.Version, buildInfo.Short())
	}
	if len(fangOpts) == 0 {
		t.Error("NewRootCmd must return the fang options the caller has to pass to fang.Execute")
	}

	for _, name := range []string{
		"out", "open", "no-open", "lang", "metric", "mode", "port",
		"keep", "no-serve", "file", "timeout", "format", "verbose",
	} {
		if root.Flags().Lookup(name) == nil {
			t.Errorf("--%s is not registered", name)
		}
	}

	var names []string
	for _, c := range root.Commands() {
		names = append(names, c.Name())
	}
	if !contains(names, "self-update") {
		t.Errorf("subcommands = %v, want self-update among them", names)
	}
}

func TestNewRootCmd_FlagDefaultsMatchTheDocumentedContract(t *testing.T) {
	root, _ := NewRootCmd(testAssets())
	flags := root.Flags()

	tests := map[string]string{
		"out":     defaultOut,
		"lang":    "all",
		"metric":  defaultMetric,
		"mode":    defaultMode,
		"port":    "5173",
		"timeout": defaultTimeout.String(),
		"format":  "text",
	}
	for name, want := range tests {
		got := flags.Lookup(name).DefValue
		if got != want {
			t.Errorf("--%s default = %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{"open": "true", "no-open": "false", "keep": "false", "no-serve": "false", "file": "false", "verbose": "false"} {
		got := flags.Lookup(name).DefValue
		if got != want {
			t.Errorf("--%s default = %q, want %q", name, got, want)
		}
	}
}

// --- self_update.go ---

func TestSelfUpdateConfig_DescribesThePublishedArtifacts(t *testing.T) {
	// cliinstall#req:catalog-identity-single-source: self-update's Config
	// is built from cover100's own catalog entry now, so this test compares
	// against that entry rather than a second, hand-duplicated set of
	// wants that could silently drift from it.
	entry, ok := cliinstall.ByID(binaryName)
	if !ok {
		t.Fatalf("no catalog entry for %q", binaryName)
	}
	cfg := selfUpdateConfig()

	if cfg.BinaryName != binaryName {
		t.Errorf("BinaryName = %q, want %q", cfg.BinaryName, binaryName)
	}
	if cfg.Repository != entry.Repository {
		t.Errorf("Repository = %q, want catalog entry Repository %q", cfg.Repository, entry.Repository)
	}
	if cfg.CurrentVersion != buildInfo.Version {
		t.Errorf("CurrentVersion = %q, want %q", cfg.CurrentVersion, buildInfo.Version)
	}
	// The probe is what proves a swapped binary actually runs, so it must be the
	// flag the root command really has.
	if len(cfg.VersionProbeArgs) != 1 || cfg.VersionProbeArgs[0] != "--version" {
		t.Errorf("VersionProbeArgs = %v, want [--version]", cfg.VersionProbeArgs)
	}
	if cfg.HTTPClient == nil {
		t.Error("HTTPClient = nil, want a bounded client so a hung endpoint cannot wedge an update")
	}
	if len(cfg.Managers) != 0 {
		t.Errorf("Managers = %v, want none: cover100 publishes only plain GitHub release archives", cfg.Managers)
	}
	// Every platform .goreleaser.yml publishes must be listed, or a supported
	// download would be refused; this now comes from the catalog entry.
	if len(cfg.SupportedPlatforms) != len(entry.SupportedPlatforms) {
		t.Errorf("SupportedPlatforms = %d entries, want %d (from the catalog entry, darwin/linux/windows x amd64/arm64)",
			len(cfg.SupportedPlatforms), len(entry.SupportedPlatforms))
	}
}

// installErrors replaced the old bespoke passthroughErrors self-update
// mapper (B1 fix: self-update, install and upgrade now share ONE mapper so
// the same failure exits the same code from all three) -- see install_test.go's
// TestInstallErrorsFailure_OtherKindsMapToFailureExitCode and
// TestInstallErrorsFailure_UpdateAvailableIsInformational for its own
// coverage.

// --- helpers.go ---

func TestResolveArg_ReportsAnUnreadableWorkingDirectory(t *testing.T) {
	original := getwd
	t.Cleanup(func() { getwd = original })
	getwd = func() (string, error) { return "", errors.New("getwd: no such file or directory") }

	// With no argument and no readable cwd there is nothing to scan, and the
	// caller has to be told rather than quietly scanning "".
	if _, err := resolveArg(nil); err == nil {
		t.Fatal("resolveArg(nil) = nil error, want a failure when the working directory is unreadable")
	}
	// An explicit path still resolves: it never needed the cwd.
	dir := t.TempDir()
	if got, err := resolveArg([]string{dir}); err != nil || got != dir {
		t.Errorf("resolveArg(%q) = (%q, %v), want (%q, nil)", dir, got, err, dir)
	}
}

func TestResolveArg_UsesTheRealWorkingDirectoryByDefault(t *testing.T) {
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveArg(nil)
	if err != nil {
		t.Fatalf("resolveArg(nil) = %v, want the working directory", err)
	}
	if got != want {
		t.Errorf("resolveArg(nil) = %q, want %q", got, want)
	}
}

// --- run.go: small helpers ---

func TestLanguageOf(t *testing.T) {
	if got := languageOf(&model.Node{Language: model.StrPtr(model.LangGo)}); got != model.LangGo {
		t.Errorf("languageOf(go) = %q, want go", got)
	}
	// The root node carries no language, and the summary table must render that
	// as an empty cell rather than a panic.
	if got := languageOf(&model.Node{}); got != "" {
		t.Errorf("languageOf(nil language) = %q, want an empty string", got)
	}
}

func TestConcurrencyFor(t *testing.T) {
	tests := []struct {
		cpus int
		want int
	}{
		{1, 2},  // a single-core machine still runs two projects at once
		{2, 2},  // the floor
		{3, 3},  // below the cap, so the core count is used
		{4, 4},  // exactly the cap
		{64, 4}, // never fork more than the cap
	}
	for _, tc := range tests {
		if got := concurrencyFor(tc.cpus); got != tc.want {
			t.Errorf("concurrencyFor(%d) = %d, want %d", tc.cpus, got, tc.want)
		}
	}
	if got := concurrency(); got < 2 || got > maxParallelProjects {
		t.Errorf("concurrency() = %d, want a value in [2, %d]", got, maxParallelProjects)
	}
}

func TestProjectLine(t *testing.T) {
	files := []model.FileCoverage{{Lines: model.Metric{Covered: 3, Total: 4}}}

	// An empty Rel is the scan root itself and must render as "." rather than as
	// a blank column.
	line := projectLine("go", "", "", files, 1500*time.Millisecond)
	if !strings.Contains(line, "go  .") {
		t.Errorf("projectLine(empty rel) = %q, want the root rendered as \".\"", line)
	}
	if !strings.Contains(line, "3/4") || !strings.Contains(line, "75.0%") {
		t.Errorf("projectLine() = %q, want the covered/total and the percentage", line)
	}
	if !strings.Contains(line, "1.5s") {
		t.Errorf("projectLine() = %q, want the elapsed time", line)
	}

	withDetail := projectLine("js", "web", "vitest", files, 10*time.Millisecond)
	if !strings.Contains(withDetail, "web (vitest)") {
		t.Errorf("projectLine() = %q, want the detail in parentheses", withDetail)
	}

	// A long repository path is truncated so the columns stay aligned.
	long := projectLine("js", strings.Repeat("very-long-repository-name/", 4), "", files, time.Second)
	if len(long) > 200 {
		t.Errorf("projectLine() did not truncate a long label: %q", long)
	}
}

// --- assemble ---

func goResult(rel string, files ...model.FileCoverage) *gocov.Result {
	return &gocov.Result{Rel: rel, Files: files}
}

func nodeResult(rel string, files ...model.FileCoverage) *tscov.Result {
	return &tscov.Result{Rel: rel, Files: files}
}

func goResultsN(n int) []*gocov.Result {
	out := make([]*gocov.Result, n)
	for i := range out {
		out[i] = &gocov.Result{Rel: "go"}
	}
	return out
}

func nodeResultsN(n int) []*tscov.Result {
	out := make([]*tscov.Result, n)
	for i := range out {
		out[i] = &tscov.Result{Rel: "web"}
	}
	return out
}

func TestAssemble_AppliesTheLanguageFilterPerFile(t *testing.T) {
	ts := model.FileCoverage{Path: "web/src/a.ts", Language: model.LangTypeScript,
		Lines: model.Metric{Covered: 1, Total: 2}}
	js := model.FileCoverage{Path: "web/src/b.js", Language: model.LangJavaScript,
		Lines: model.Metric{Covered: 2, Total: 2}}
	goFile := model.FileCoverage{Path: "pkg/a.go", Language: model.LangGo,
		Lines: model.Metric{Covered: 1, Total: 1}}

	tests := []struct {
		name      string
		lang      string
		wantPaths []string
	}{
		{"all keeps every language", "all", []string{"pkg/a.go", "web/src/a.ts", "web/src/b.js"}},
		{"ts keeps only TypeScript files", "ts", []string{"pkg/a.go", "web/src/a.ts"}},
		{"js keeps only JavaScript files", "js", []string{"pkg/a.go", "web/src/b.js"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projects, warnings := assemble(
				[]*gocov.Result{goResult("goproj", goFile)},
				[]*tscov.Result{nodeResult("web", ts, js)},
				tc.lang,
			)
			if len(warnings) != 0 {
				t.Errorf("warnings = %v, want none", warnings)
			}

			var got []string
			for _, p := range projects {
				for _, f := range p.Files {
					got = append(got, f.Path)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.wantPaths, ",") {
				t.Errorf("assemble(%q) kept %v, want %v", tc.lang, got, tc.wantPaths)
			}
		})
	}
}

func TestAssemble_MergesProjectsThatShareARepositoryDirectory(t *testing.T) {
	goFile := model.FileCoverage{Path: "a.go", Language: model.LangGo, Lines: model.Metric{Covered: 1, Total: 1}}
	ts := model.FileCoverage{Path: "src/a.ts", Language: model.LangTypeScript, Lines: model.Metric{Covered: 1, Total: 1}}

	projects, _ := assemble(
		[]*gocov.Result{goResult(".", goFile)},
		[]*tscov.Result{nodeResult(".", ts)},
		"all",
	)

	if len(projects) != 1 {
		t.Fatalf("assemble() produced %d projects, want 1 merged repository: %+v", len(projects), projects)
	}
	if len(projects[0].Files) != 2 {
		t.Errorf("merged project has %d files, want 2", len(projects[0].Files))
	}
}

func TestAssemble_IgnoresNilResultsAndEmptyFileLists(t *testing.T) {
	projects, warnings := assemble(
		[]*gocov.Result{nil, goResult("goproj")},
		[]*tscov.Result{nil, nodeResult("web")},
		"all",
	)
	if len(projects) != 0 {
		t.Errorf("projects = %+v, want none for nil and empty results", projects)
	}
	if warnings != nil {
		t.Errorf("warnings = %v, want nil", warnings)
	}
}

func TestAssemble_WarnsWhenTheFilterEmptiesAPackage(t *testing.T) {
	onlyJS := model.FileCoverage{Path: "web/src/b.js", Language: model.LangJavaScript,
		Lines: model.Metric{Covered: 1, Total: 1}}

	projects, warnings := assemble(nil, []*tscov.Result{nodeResult("web", onlyJS)}, "ts")

	if len(projects) != 0 {
		t.Errorf("projects = %+v, want the JavaScript-only package excluded", projects)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no ts coverage found") {
		t.Errorf("warnings = %v, want one explaining the exclusion", warnings)
	}
}

func TestAssemble_CollectsWarningsFromEveryResult(t *testing.T) {
	goRes := goResult("goproj")
	goRes.Warnings = []string{"go warning"}
	nodeRes := nodeResult("web")
	nodeRes.Warnings = []string{"node warning"}

	_, warnings := assemble([]*gocov.Result{goRes}, []*tscov.Result{nodeRes}, "all")

	if !hasSubstring(warnings, "go warning") || !hasSubstring(warnings, "node warning") {
		t.Errorf("warnings = %v, want both collectors' warnings", warnings)
	}
}

// --- cleanupIntermediates ---

func TestCleanupIntermediates_RemovesTrackedArtefactsAndKeepsTheReport(t *testing.T) {
	workDir := t.TempDir()
	profile := filepath.Join(workDir, "go-0.out")
	tsDir := filepath.Join(workDir, "ts-0")
	report := filepath.Join(workDir, "coverage.json")
	viewer := filepath.Join(workDir, "view.html")

	for _, p := range []string{profile, report, viewer} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(tsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cleanupIntermediates(ui.New(io.Discard, io.Discard, false), goResultsN(1), nodeResultsN(1), workDir, false)

	for _, gone := range []string{profile, tsDir} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s survived cleanup", gone)
		}
	}
	// The report and the standalone viewer live in the same directory by
	// default, so deleting them would destroy the run's own output.
	for _, kept := range []string{report, viewer} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was deleted by cleanup: %v", kept, err)
		}
	}
}

func TestCleanupIntermediates_KeepRetainsEverything(t *testing.T) {
	workDir := t.TempDir()
	profile := filepath.Join(workDir, "go-0.out")
	if err := os.WriteFile(profile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanupIntermediates(ui.New(io.Discard, io.Discard, false), goResultsN(1), nil, workDir, true)

	if _, err := os.Stat(profile); err != nil {
		t.Errorf("--keep must retain %s: %v", profile, err)
	}
}

func TestCleanupIntermediates_MissingArtefactsAreNotAnError(t *testing.T) {
	// A collector that never wrote its profile must not make cleanup fail.
	cleanupIntermediates(ui.New(io.Discard, io.Discard, true), goResultsN(2), nodeResultsN(2), t.TempDir(), false)
}

func TestCleanupIntermediates_LogsWhatItCannotRemove(t *testing.T) {
	workDir := t.TempDir()
	// The Go profile is removed with os.Remove, which refuses a non-empty
	// directory. The failure must be logged and swallowed, never surfaced as a
	// run failure.
	stuck := filepath.Join(workDir, "go-0.out")
	if err := os.MkdirAll(filepath.Join(stuck, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	var verbose strings.Builder
	printer := ui.New(io.Discard, &verbose, true)

	cleanupIntermediates(printer, goResultsN(1), nil, workDir, false)

	// Only the artefact's identity is asserted: the reason text is the
	// operating system's ("directory not empty" on Unix, "The directory is not
	// empty." on Windows).
	if !strings.Contains(verbose.String(), "go-0.out") {
		t.Errorf("verbose log = %q, want it to name the artefact it could not remove", verbose.String())
	}
}

// --- encodeSummary ---

func TestEncodeSummary_ReportsAWriteFailureAsUnexpected(t *testing.T) {
	cmd := &cobra.Command{Use: binaryName}
	cmd.SetOut(failingWriter{})

	err := encodeSummary(cmd, summary{Tool: "cover100 test"})
	if err == nil {
		t.Fatal("encodeSummary() = nil, want an error when the writer refuses")
	}
	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
}

// --- collectAll ---

func TestCollectAll_RunsBothLanguagesAndNamesTheAssumedRunner(t *testing.T) {
	requirePOSIXStubs(t)

	root := t.TempDir()
	goDir := filepath.Join(root, "goproj")
	nodeDir := filepath.Join(root, "web")
	for _, dir := range []string{goDir, nodeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	goBin := stubCommand(t, "go", `for arg in "$@"; do
  case "$arg" in -coverprofile=*) out="${arg#-coverprofile=}" ;; esac
done
cat > "$out" <<'PROFILE'
mode: set
example.com/x/pkg/a.go:1.1,2.1 1 1
example.com/x/pkg/a.go:5.1,6.1 1 0
PROFILE`)
	t.Setenv("PATH", goBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	jestBin := filepath.Join(nodeDir, "node_modules", ".bin")
	if err := os.MkdirAll(jestBin, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(nodeDir, "src", "a.ts")
	writeStubAt(t, filepath.Join(jestBin, "jest"), `for arg in "$@"; do
  case "$arg" in --coverageDirectory=*) out="${arg#--coverageDirectory=}" ;; esac
done
mkdir -p "$out"
cat > "$out/coverage-final.json" <<JSON
{"`+tsFile+`":{"path":"`+tsFile+`","statementMap":{"0":{"start":{"line":1,"column":0},"end":{"line":1,"column":5}}},"fnMap":{},"branchMap":{},"s":{"0":1},"f":{},"b":{}}}
JSON`)

	goProjects := []detect.Project{{
		Dir: goDir, Rel: "goproj", Kind: detect.KindGo, ModulePath: "example.com/x",
	}}
	// Runner "" is the branch under test: tscov must assume jest and say so.
	nodeProjects := []detect.Project{{
		Dir: nodeDir, Rel: "web", Kind: detect.KindNode,
		PackageName: "web", TestScript: "jest", HasTestScript: true, Runner: "",
	}}

	goResults, nodeResults := collectAll(context.Background(),
		ui.New(io.Discard, io.Discard, false), root, t.TempDir(), goProjects, nodeProjects, time.Minute)

	if len(goResults) != 1 || len(nodeResults) != 1 {
		t.Fatalf("collectAll returned %d go and %d node results, want 1 and 1", len(goResults), len(nodeResults))
	}
	if len(goResults[0].Files) != 1 {
		t.Errorf("go files = %+v, want the stubbed profile parsed", goResults[0].Files)
	}
	if len(nodeResults[0].Files) != 1 {
		t.Errorf("node files = %+v, want the stubbed report parsed", nodeResults[0].Files)
	}
	if nodeResults[0].Runner != "jest" {
		t.Errorf("runner = %q, want jest when the project declares none", nodeResults[0].Runner)
	}
	if !hasSubstring(nodeResults[0].Warnings, "no test runner dependency") {
		t.Errorf("warnings = %v, want one naming the assumed runner", nodeResults[0].Warnings)
	}
}

// --- runCollect ---

// failingSubFS models an asset tree that cannot provide the requested
// sub-directory, which is what embed.FS does when the go:embed directive does
// not actually cover public/.
type failingSubFS struct{ err error }

func (f failingSubFS) Open(string) (fs.File, error) { return nil, f.err }
func (f failingSubFS) Sub(string) (fs.FS, error)    { return nil, f.err }

func TestRunCollect_RequiresThePublicAssetSubtree(t *testing.T) {
	// A binary whose embed directive lost public/ must fail loudly rather than
	// serve a blank page.
	err := runCollect(context.Background(), quietCommand(), []string{t.TempDir()},
		failingSubFS{err: fs.ErrNotExist}, collectOptsFor(filepath.Join(t.TempDir(), "coverage.json")))

	if err == nil {
		t.Fatal("runCollect() = nil, want an error when public/ is missing from the assets")
	}
	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
}

func TestRunCollect_StopsWhenInterrupted(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // interrupt before collection starts

	err := runCollect(ctx, quietCommand(), []string{fixture},
		testAssets(), collectOptsFor(filepath.Join(t.TempDir(), "coverage.json")))

	if err == nil {
		t.Fatal("runCollect() = nil, want an interruption error")
	}
	if got := exitCodeOfError(t, err); got != 130 {
		t.Errorf("exit code = %d, want 130", got)
	}
}

func TestRunCollect_FailsWhenTheReportCannotBeWritten(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)

	// --out pointing at an existing directory makes the write fail after an
	// otherwise successful collection.
	blocked := filepath.Join(t.TempDir(), "coverage.json")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}

	err := runCollect(context.Background(), quietCommand(), []string{fixture},
		testAssets(), collectOptsFor(blocked))

	if err == nil {
		t.Fatal("runCollect() = nil, want an error when the report cannot be written")
	}
	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
}

func TestRunCollect_WarnsWhenTheStandaloneViewerCannotBeBuilt(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)
	outDir := t.TempDir()
	reportPath := filepath.Join(outDir, "coverage.json")

	// public/index.html exists but its siblings do not, so inlining fails while
	// the served page would still work: a warning, not a failed run.
	assets := fstest.MapFS{
		"public/index.html": &fstest.MapFile{Data: []byte(
			`<html><head><link rel="stylesheet" href="styles.css"></head><body><script src="app.js"></script></body></html>`)},
	}

	var stderr strings.Builder
	cmd := quietCommand()
	cmd.SetErr(&stderr)

	if err := runCollect(context.Background(), cmd, []string{fixture}, assets,
		collectOptsFor(reportPath)); err != nil {
		t.Fatalf("runCollect() = %v, want nil: a missing viewer asset must not fail the run", err)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("the report must still be written: %v", err)
	}
	if !strings.Contains(stderr.String(), "standalone report") {
		t.Errorf("stderr = %q, want a warning about the standalone report", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, standaloneName)); err == nil {
		t.Error("view.html must not exist when inlining failed")
	}
}

func TestRunCollect_WritesTheReportAndTheStandaloneViewer(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)
	outDir := t.TempDir()
	reportPath := filepath.Join(outDir, "coverage.json")

	var stdout strings.Builder
	cmd := quietCommand()
	cmd.SetOut(&stdout)

	if err := runCollect(context.Background(), cmd, []string{fixture}, testAssets(),
		collectOptsFor(reportPath)); err != nil {
		t.Fatalf("runCollect() = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "wrote "+reportPath) {
		t.Errorf("stdout = %q, want it to name the report it wrote", stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+filepath.Join(outDir, standaloneName)) {
		t.Errorf("stdout = %q, want it to name the standalone viewer it wrote", stdout.String())
	}
}

func TestRunCollect_NoServeWithJSONEmitsOnlyTheSummary(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)
	outDir := t.TempDir()

	opts := collectOptsFor(filepath.Join(outDir, "coverage.json"))
	opts.format = "json"

	var stdout strings.Builder
	cmd := quietCommand()
	cmd.SetOut(&stdout)

	if err := runCollect(context.Background(), cmd, []string{fixture}, testAssets(), opts); err != nil {
		t.Fatalf("runCollect() = %v, want nil", err)
	}

	var sum summary
	if err := json.Unmarshal([]byte(stdout.String()), &sum); err != nil {
		t.Fatalf("stdout is not one JSON document: %v (%q)", err, stdout.String())
	}
	if sum.Report == "" || len(sum.Metrics) != 5 {
		t.Errorf("summary = %+v, want a report path and all five metrics", sum)
	}
}

func TestRunCollect_FileModeWritesAndNamesTheStandaloneReport(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)
	outDir := t.TempDir()

	opts := collectOptsFor(filepath.Join(outDir, "coverage.json"))
	opts.noServe = false
	opts.file = true

	var stdout strings.Builder
	cmd := quietCommand()
	cmd.SetOut(&stdout)

	if err := runCollect(context.Background(), cmd, []string{fixture}, testAssets(), opts); err != nil {
		t.Fatalf("runCollect(--file) = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "file://") {
		t.Errorf("stdout = %q, want the file:// URL of the standalone report", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, standaloneName)); err != nil {
		t.Errorf("--file must write the standalone report: %v", err)
	}
}

// TestRunCollect_FileModeWarnsWhenNoLauncherExists covers the wantsOpen branch
// without ever reaching a browser: PATH holds only a stub `go`, so
// serve.OpenBrowser cannot find a launcher and returns an error.
func TestRunCollect_FileModeWarnsWhenNoLauncherExists(t *testing.T) {
	requirePOSIXStubs(t)

	fixture := t.TempDir()
	writeTestFile(t, filepath.Join(fixture, "go.mod"), "module example.com/fixture\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(fixture, "a.go"), "package fixture\n")

	// A stub `go` writes a profile and nothing else; with this directory as the
	// whole PATH there is no `open`, `xdg-open` or `rundll32` to find.
	stub := stubCommand(t, "go", `for arg in "$@"; do
  case "$arg" in -coverprofile=*) out="${arg#-coverprofile=}" ;; esac
done
cat > "$out" <<'PROFILE'
mode: set
example.com/fixture/a.go:1.1,2.1 1 1
PROFILE`)
	t.Setenv("PATH", stub)
	if _, err := exec.LookPath("open"); err == nil {
		t.Skip("a launcher is still reachable; skipping so no browser can be opened")
	}

	outDir := t.TempDir()
	opts := collectOptsFor(filepath.Join(outDir, "coverage.json"))
	opts.noServe = false
	opts.file = true
	opts.noOpen = false
	opts.open = true

	var stderr strings.Builder
	cmd := quietCommand()
	cmd.SetErr(&stderr)

	if err := runCollect(context.Background(), cmd, []string{fixture}, testAssets(), opts); err != nil {
		t.Fatalf("runCollect() = %v, want nil: a missing launcher is not a run failure", err)
	}
	if !strings.Contains(stderr.String(), "opening") {
		t.Errorf("stderr = %q, want a warning that the report could not be opened", stderr.String())
	}
}

// --- serveReport ---

// stubServer is a reportServer whose terminal path a test chooses.
type stubServer struct {
	serveErr error
	closed   bool
}

func (s *stubServer) PageURL(metric, mode string) string {
	return "http://127.0.0.1:1/?data=/coverage.json&metric=" + metric + "&mode=" + mode
}
func (s *stubServer) Serve() error { return s.serveErr }
func (s *stubServer) Close() error { s.closed = true; return nil }

// withStubServer swaps the start seam for the duration of a test.
func withStubServer(t *testing.T, srv *stubServer, err error) *stubServer {
	t.Helper()
	original := startReportServer
	t.Cleanup(func() { startReportServer = original })
	startReportServer = func(serve.Options) (reportServer, error) { return srv, err }
	return srv
}

// blockingServer never returns from Serve, modelling a live listener.
type blockingServer struct{}

func (blockingServer) PageURL(string, string) string { return "http://127.0.0.1:1/" }
func (blockingServer) Serve() error                  { select {} }
func (blockingServer) Close() error                  { return nil }

func TestServeReport_ReportsAStartFailure(t *testing.T) {
	withStubServer(t, nil, errors.New("address already in use"))

	err := serveReport(context.Background(), quietCommand(), ui.New(io.Discard, io.Discard, false),
		collectOptsFor("out.json"), testAssets(), "out.json", summary{})

	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
}

func TestServeReport_ReturnsWhenServingStopsCleanly(t *testing.T) {
	srv := withStubServer(t, &stubServer{}, nil)

	err := serveReport(context.Background(), quietCommand(), ui.New(io.Discard, io.Discard, false),
		collectOptsFor("out.json"), testAssets(), "out.json", summary{})

	if err != nil {
		t.Errorf("serveReport() = %v, want nil when Serve returns without an error", err)
	}
	if !srv.closed {
		t.Error("the server was not closed")
	}
}

func TestServeReport_ReportsAServingFailure(t *testing.T) {
	withStubServer(t, &stubServer{serveErr: errors.New("listener died")}, nil)

	err := serveReport(context.Background(), quietCommand(), ui.New(io.Discard, io.Discard, false),
		collectOptsFor("out.json"), testAssets(), "out.json", summary{})

	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
}

func TestServeReport_BlocksUntilInterrupted(t *testing.T) {
	original := startReportServer
	t.Cleanup(func() { startReportServer = original })
	startReportServer = func(serve.Options) (reportServer, error) { return blockingServer{}, nil }

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var stdout strings.Builder
	err := serveReport(ctx, quietCommand(), ui.New(&stdout, io.Discard, false),
		collectOptsFor("out.json"), testAssets(), "out.json", summary{})

	if got := exitCodeOfError(t, err); got != 130 {
		t.Errorf("exit code = %d, want 130 on interrupt", got)
	}
	if !strings.Contains(stdout.String(), "shutting down") {
		t.Errorf("stdout = %q, want it to announce the shutdown", stdout.String())
	}
}

func TestServeReport_JSONModeEmitsTheSummaryBeforeServing(t *testing.T) {
	withStubServer(t, &stubServer{}, nil)

	opts := collectOptsFor("out.json")
	opts.format = "json"
	var stdout strings.Builder
	cmd := quietCommand()
	cmd.SetOut(&stdout)

	if err := serveReport(context.Background(), cmd, ui.New(io.Discard, io.Discard, false),
		opts, testAssets(), "out.json", summary{Tool: "cover100 test"}); err != nil {
		t.Fatalf("serveReport() = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), `"tool": "cover100 test"`) {
		t.Errorf("stdout = %q, want the JSON summary", stdout.String())
	}
}

func TestServeReport_JSONModePropagatesASummaryWriteFailure(t *testing.T) {
	withStubServer(t, &stubServer{}, nil)

	opts := collectOptsFor("out.json")
	opts.format = "json"
	cmd := &cobra.Command{Use: binaryName}
	cmd.SetOut(failingWriter{})

	err := serveReport(context.Background(), cmd, ui.New(io.Discard, io.Discard, false),
		opts, testAssets(), "out.json", summary{})

	if got := exitCodeOfError(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d", got, exitcode.Unexpected)
	}
}

// TestServeReport_WarnsWhenNoBrowserLauncherExists covers the wantsOpen branch
// without reaching a browser: PATH is emptied, so the launcher cannot be found.
func TestServeReport_WarnsWhenNoBrowserLauncherExists(t *testing.T) {
	withStubServer(t, &stubServer{}, nil)
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("open"); err == nil {
		t.Skip("a launcher is still reachable; skipping so no browser can be opened")
	}

	opts := collectOptsFor("out.json")
	opts.noOpen = false
	opts.open = true
	var stderr strings.Builder

	if err := serveReport(context.Background(), quietCommand(), ui.New(io.Discard, &stderr, false),
		opts, testAssets(), "out.json", summary{}); err != nil {
		t.Fatalf("serveReport() = %v, want nil: a missing launcher is not fatal", err)
	}
	if !strings.Contains(stderr.String(), "opening") {
		t.Errorf("stderr = %q, want a warning that the page could not be opened", stderr.String())
	}
}
