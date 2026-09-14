package gocov

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/ui"
)

// fakeGoWithProfileDir installs a stub `go` that creates a *directory* where
// the coverage profile should be. os.Open succeeds on a directory but reading
// it fails, so this reaches Run's profile-parse-error branch through a real
// file descriptor rather than a hand-built reader.
func fakeGoWithProfileDir(t *testing.T) {
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
if [ -n "$out" ]; then mkdir -p "$out"; fi
echo "stub go ran"
exit 0
`
	path := filepath.Join(binDir, "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRun_WarnsWhenTheProfileCannotBeParsed(t *testing.T) {
	fakeGoWithProfileDir(t)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err == nil {
		t.Fatal("Run() must report the profile parse failure")
	}
	if len(res.Files) != 0 {
		t.Errorf("Files = %+v, want none after a parse failure", res.Files)
	}
	if !hasWarning(res.Warnings, "parsing coverage profile") {
		t.Errorf("Warnings = %v, want one naming the parse failure", res.Warnings)
	}
}

func TestRun_LogsToAVerbosePrinter(t *testing.T) {
	fakeGo(t, `mode: set
example.com/fixture/pkg/calc.go:1.1,2.1 1 1`, 0)

	root, moduleDir := moduleFixture(t)
	var diagnostics strings.Builder
	printer := ui.New(io.Discard, &diagnostics, true)

	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(), Printer: printer,
	}, 0)

	if res.Err != nil {
		t.Fatalf("Run() error = %v", res.Err)
	}
	if !strings.Contains(diagnostics.String(), "running go test") {
		t.Errorf("verbose printer output = %q, want it to record the command being run", diagnostics.String())
	}
}

func TestRun_SortsFilesByPath(t *testing.T) {
	fakeGo(t, `mode: set
example.com/fixture/pkg/c.go:1.1,2.1 1 1
example.com/fixture/pkg/a.go:1.1,2.1 1 1
example.com/fixture/pkg/b.go:1.1,2.1 1 0`, 0)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	var paths []string
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	want := []string{"services/api/pkg/a.go", "services/api/pkg/b.go", "services/api/pkg/c.go"}
	if len(paths) != len(want) {
		t.Fatalf("Files = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("Files = %v, want %v (sorted by path)", paths, want)
		}
	}
}

// TestRun_CountsEntriesThatEscapeTheScanRootAsSkipped covers the second skip
// reason: the entry belongs to the module but resolves to a path outside the
// scan root, such as a `..` climb in a profile path.
func TestRun_CountsEntriesThatEscapeTheScanRootAsSkipped(t *testing.T) {
	fakeGo(t, `mode: set
example.com/fixture/pkg/calc.go:1.1,2.1 1 1
example.com/fixture/../../../outside/escaped.go:1.1,2.1 1 1`, 0)

	root, moduleDir := moduleFixture(t)
	res := Run(context.Background(), projectFor(root, moduleDir), Options{
		Root: root, WorkDir: t.TempDir(),
	}, 0)

	if res.Err != nil {
		t.Fatalf("Run() error = %v", res.Err)
	}
	if len(res.Files) != 1 || res.Files[0].Path != "services/api/pkg/calc.go" {
		t.Fatalf("Files = %+v, want only the file inside the scan root", res.Files)
	}
	if !hasWarning(res.Warnings, "ignored 1 coverage entries outside the module directory") {
		t.Errorf("Warnings = %v, want the skipped-entry count", res.Warnings)
	}
}

// TestParseProfileRejectsLineRangesThatRunBackwards checks the clamp: a
// malformed block whose end precedes its start must still record one line, not
// silently disappear or crash the loop.
func TestParseProfileRejectsLineRangesThatRunBackwards(t *testing.T) {
	files, err := ParseProfile(strings.NewReader("mode: set\nexample.com/x/a.go:10.1,5.2 1 1\n"))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	lines := files["example.com/x/a.go"]
	if len(lines) != 1 {
		t.Fatalf("parsed lines = %v, want only line 10: a backwards range is clamped to its start", lines)
	}
	if lines[10] != 1 {
		t.Errorf("line 10 count = %d, want 1", lines[10])
	}
	if _, ok := lines[5]; ok {
		t.Error("line 5 must not be recorded: the end line is clamped up, never down")
	}
}

func TestParseProfileToleratesHeaderLinesAnywhere(t *testing.T) {
	profile := "mode: set\nexample.com/x/a.go:1.1,2.1 1 1\nmode: count\nexample.com/x/b.go:1.1,2.1 1 0\n"
	files, err := ParseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("parsed %d files, want 2: a mode: line in the middle must be skipped, not parsed", len(files))
	}
	if got := files["example.com/x/a.go"][1]; got != 1 {
		t.Errorf("a.go line 1 count = %d, want 1", got)
	}
	if got := files["example.com/x/b.go"][1]; got != 0 {
		t.Errorf("b.go line 1 count = %d, want 0 (an uncovered block must survive)", got)
	}
}

func TestParseProfileHandlesCRLFProfiles(t *testing.T) {
	files, err := ParseProfile(strings.NewReader("mode: set\r\nexample.com/x/a.go:1.1,2.1 1 1\r\n"))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	lines := files["example.com/x/a.go"]
	if lines == nil {
		t.Fatal("a CRLF profile line was not parsed; the trailing carriage return must be trimmed")
	}
	if lines[1] != 1 || lines[2] != 1 {
		t.Errorf("a.go lines = %v, want lines 1 and 2 covered", lines)
	}
}

func TestParseProfileOfAHeaderOnlyProfile(t *testing.T) {
	files, err := ParseProfile(strings.NewReader("mode: set\n"))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("parsed %d files from a header-only profile, want 0", len(files))
	}
}

func TestParseProfileOfOnlyUnknownLines(t *testing.T) {
	files, err := ParseProfile(strings.NewReader("mode: set\nnot a profile record\n"))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("parsed %d files from a profile of unrecognised lines, want 0", len(files))
	}
}

// failingReader yields data once and then fails, which is the only way to make
// bufio.Scanner report an error through ParseProfile's io.Reader interface.
type failingReader struct {
	data string
	err  error
	done bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), nil
}

func TestParseProfilePropagatesReadErrors(t *testing.T) {
	boom := errors.New("read boom")
	files, err := ParseProfile(&failingReader{data: "mode: set\n", err: boom})
	if err == nil {
		t.Fatal("ParseProfile() must return the reader's error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("ParseProfile() error = %v, want it to wrap %v", err, boom)
	}
	if files != nil {
		t.Errorf("ParseProfile() files = %v on error, want nil", files)
	}
}

func TestAtoi(t *testing.T) {
	if got := atoi("42"); got != 42 {
		t.Errorf("atoi(\"42\") = %d, want 42", got)
	}
	if got := atoi("not-a-number"); got != 0 {
		t.Errorf("atoi(\"not-a-number\") = %d, want 0: an unparsable field degrades to zero", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate(\"short\", 10) = %q, want it unchanged", got)
	}

	exact := strings.Repeat("a", 10)
	if got := truncate(exact, 10); got != exact {
		t.Errorf("truncate of an exactly-at-cap string = %q, want it unchanged", got)
	}

	over := strings.Repeat("b", 25)
	want := strings.Repeat("b", 10) + "\n... (output truncated)"
	if got := truncate(over, 10); got != want {
		t.Errorf("truncate of an over-cap string = %q, want %q", got, want)
	}
}

func TestExitCodeOfANonExitError(t *testing.T) {
	if got := exitCode(errors.New("spawn failed")); got != -1 {
		t.Errorf("exitCode(non-exit error) = %d, want -1: only exec.ExitError carries an exit status", got)
	}
}
