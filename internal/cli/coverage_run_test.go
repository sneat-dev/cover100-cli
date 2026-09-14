package cli

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sneat-dev/cover100-cli/internal/serve"
	"github.com/sneat-dev/cover100-cli/internal/ui"
)

// failingOutCommand is a command whose stdout refuses every write, so the
// summary-encoding failures can be provoked.
func failingOutCommand() *cobra.Command {
	cmd := &cobra.Command{Use: binaryName, SilenceUsage: true, SilenceErrors: true}
	cmd.SetOut(failingWriter{})
	cmd.SetErr(io.Discard)
	return cmd
}

// This file covers runCollect's failure and diagnostic branches: the paths that
// only a broken environment, an unwritable output location, or a failing
// encoder can reach.

func TestRunCollect_ReportsAnUnresolvableScanPath(t *testing.T) {
	original := getwd
	t.Cleanup(func() { getwd = original })
	getwd = func() (string, error) { return "", errors.New("getwd: no such file or directory") }

	// No positional argument, so the scan root would have to come from the
	// working directory, which is unreadable.
	err := runCollect(context.Background(), quietCommand(), nil,
		testAssets(), collectOptsFor(filepath.Join(t.TempDir(), "coverage.json")))

	if err == nil {
		t.Fatal("runCollect() = nil, want an error when the scan path cannot be resolved")
	}
	if got := exitCodeOfError(t, err); got != 10 {
		t.Errorf("exit code = %d, want 10", got)
	}
	if !strings.Contains(err.Error(), "resolving the scan path") {
		t.Errorf("error = %q, want it to name the failing step", err)
	}
}

func TestRunCollect_ReportsAnUnresolvableOutputPath(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)

	original := absPath
	t.Cleanup(func() { absPath = original })
	absPath = func(string) (string, error) { return "", errors.New("no working directory") }

	err := runCollect(context.Background(), quietCommand(), []string{fixture},
		testAssets(), collectOptsFor("relative/coverage.json"))

	if err == nil {
		t.Fatal("runCollect() = nil, want an error when --out cannot be resolved")
	}
	if !strings.Contains(err.Error(), "resolving --out") {
		t.Errorf("error = %q, want it to name --out", err)
	}
}

func TestRunCollect_ReportsAnUncreatableOutputDirectory(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)

	// A regular file where the output directory should be: MkdirAll cannot
	// create a directory beneath it.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	writeTestFile(t, blocker, "x")

	err := runCollect(context.Background(), quietCommand(), []string{fixture},
		testAssets(), collectOptsFor(filepath.Join(blocker, "nested", "coverage.json")))

	if err == nil {
		t.Fatal("runCollect() = nil, want an error when the output directory cannot be created")
	}
	if !strings.Contains(err.Error(), "creating") {
		t.Errorf("error = %q, want it to name the directory it could not create", err)
	}
}

func TestRunCollect_ReportsAnUnencodableReport(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)

	original := marshalReport
	t.Cleanup(func() { marshalReport = original })
	marshalReport = func(any, string, string) ([]byte, error) {
		return nil, errors.New("encoder refused")
	}

	err := runCollect(context.Background(), quietCommand(), []string{fixture},
		testAssets(), collectOptsFor(filepath.Join(t.TempDir(), "coverage.json")))

	if err == nil {
		t.Fatal("runCollect() = nil, want an error when the report cannot be encoded")
	}
	if !strings.Contains(err.Error(), "encoding the report") {
		t.Errorf("error = %q, want it to name the failing step", err)
	}
}

func TestRunCollect_WarnsWhenTheStandaloneViewerCannotBeWritten(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)
	outDir := t.TempDir()

	// A directory where view.html should go makes the write fail after inlining
	// succeeded, which is a warning rather than a failed run.
	if err := os.MkdirAll(filepath.Join(outDir, standaloneName), 0o755); err != nil {
		t.Fatal(err)
	}

	var stderr strings.Builder
	cmd := quietCommand()
	cmd.SetErr(&stderr)

	if err := runCollect(context.Background(), cmd, []string{fixture},
		testAssets(), collectOptsFor(filepath.Join(outDir, "coverage.json"))); err != nil {
		t.Fatalf("runCollect() = %v, want nil: an unwritable viewer is not a run failure", err)
	}
	if !strings.Contains(stderr.String(), "could not write") {
		t.Errorf("stderr = %q, want a warning that the viewer could not be written", stderr.String())
	}
}

func TestRunCollect_NamesTheNodePackageItFound(t *testing.T) {
	requirePOSIXStubs(t)

	// A Node package with a test script, so the detection summary reports it.
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "package.json"),
		`{"name":"web","scripts":{"test":"vitest run"},"devDependencies":{"vitest":"^5"}}`)
	binDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The stub writes no report, which is fine: this test is about the summary.
	writeStubAt(t, filepath.Join(binDir, "vitest"), "exit 0")

	var stdout strings.Builder
	cmd := quietCommand()
	cmd.SetOut(&stdout)

	if err := runCollect(context.Background(), cmd, []string{dir},
		testAssets(), collectOptsFor(filepath.Join(t.TempDir(), "coverage.json"))); err != nil {
		t.Fatalf("runCollect() = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "1 Node package with a test script") {
		t.Errorf("stdout = %q, want the Node package counted in the summary", stdout.String())
	}
}

func TestRunCollect_FileModeJSONReportsASummaryWriteFailure(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)

	opts := collectOptsFor(filepath.Join(t.TempDir(), "coverage.json"))
	opts.noServe = false
	opts.file = true
	opts.format = "json"

	err := runCollect(context.Background(), failingOutCommand(), []string{fixture}, testAssets(), opts)
	if err == nil {
		t.Fatal("runCollect() = nil, want the summary write failure to propagate")
	}
	if !strings.Contains(err.Error(), "encoding the summary") {
		t.Errorf("error = %q, want it to name the failing step", err)
	}
}

func TestRunCollect_HandsOffToTheServer(t *testing.T) {
	requireGoToolchain(t)
	fixture := writeGoFixture(t)

	srv := withStubServer(t, &stubServer{}, nil)

	opts := collectOptsFor(filepath.Join(t.TempDir(), "coverage.json"))
	opts.noServe = false
	opts.file = false
	opts.noOpen = true

	var stdout strings.Builder
	cmd := quietCommand()
	cmd.SetOut(&stdout)

	if err := runCollect(context.Background(), cmd, []string{fixture}, testAssets(), opts); err != nil {
		t.Fatalf("runCollect() = %v, want nil once serving stops cleanly", err)
	}
	if !srv.closed {
		t.Error("the server was not closed")
	}
	if !strings.Contains(stdout.String(), "serving http://127.0.0.1:1/") {
		t.Errorf("stdout = %q, want the served URL", stdout.String())
	}
}

func TestStartReportServer_BindsARealListener(t *testing.T) {
	// The seam's default implementation must stay exercised: every other test
	// replaces it, so nothing else proves the real server can start.
	public, err := fs.Sub(testAssets(), "public")
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(t.TempDir(), "coverage.json")
	writeTestFile(t, dataPath, `{"ok":true}`)

	srv, err := startReportServer(serve.Options{Assets: public, DataPath: dataPath, Port: 0})
	if err != nil {
		t.Fatalf("startReportServer() = %v, want a live server", err)
	}
	defer func() { _ = srv.Close() }()

	if page := srv.PageURL("lines", "percent"); !strings.HasPrefix(page, "http://127.0.0.1:") {
		t.Errorf("PageURL() = %q, want a loopback URL", page)
	}
	go func() { _ = srv.Serve() }()
}

func TestCleanupIntermediates_LogsACoverageDirectoryItCannotRemove(t *testing.T) {
	workDir := t.TempDir()
	// RemoveAll fails only when it cannot unlink an entry, which needs the
	// entry's *parent* to be unwritable: a directory's own mode does not stop
	// it being removed. That is the only way to reach the reporting branch for
	// the ts-* directories.
	sealed := filepath.Join(workDir, "ts-0", "sealed")
	if err := os.MkdirAll(sealed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sealed, "entry"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0o500); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(sealed, "entry")
	t.Cleanup(func() {
		_ = os.Chmod(sealed, 0o755)
		_ = os.Remove(entry)
	})

	// Prove the protection actually holds before asserting on the failure:
	// Windows and root ignore mode bits, so the branch is unreachable there and
	// a skip is more honest than a green test that proves nothing.
	if err := os.Remove(entry); err == nil {
		t.Skip("permission bits are not enforced here; cannot make RemoveAll fail")
	}

	var verbose strings.Builder
	cleanupIntermediates(ui.New(io.Discard, &verbose, true), nil, nodeResultsN(1), workDir, false)

	if !strings.Contains(verbose.String(), "ts-0") {
		t.Errorf("verbose log = %q, want it to name the directory it could not remove", verbose.String())
	}
}
