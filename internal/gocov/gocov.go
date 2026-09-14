// Package gocov collects Go coverage by running `go test -coverprofile` in
// each detected module and parsing the resulting cover profile directly.
//
// No external conversion tool is required: the profile's own `mode:` header
// and `file:startLine.startCol,endLine.endCol numStmts count` records carry
// everything the normalized model needs.
package gocov

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sneat-dev/cover100-cli/internal/detect"
	"github.com/sneat-dev/cover100-cli/internal/model"
	"github.com/sneat-dev/cover100-cli/internal/pathutil"
	"github.com/sneat-dev/cover100-cli/internal/ui"
)

// maxOutputBytes caps how much captured `go test` output is retained for
// verbosity and failure reporting, so one pathological module cannot exhaust
// memory.
const maxOutputBytes = 256 << 10

// profileRecord matches one cover profile line:
//
//	github.com/foo/bar/file.go:10.20,12.3 1 1
//
// The leading group is greedy so paths containing colons (Windows drive
// letters, unusual module paths) still parse: the numeric tail is what anchors
// the match.
var profileRecord = regexp.MustCompile(`^(.+):(\d+)\.(\d+),(\d+)\.(\d+)[ \t]+(\d+)[ \t]+(\d+)\s*$`)

// Options configures a collection run.
type Options struct {
	// Root is the absolute scan root, used to make reported paths relative.
	Root string
	// WorkDir receives the intermediate .out profile.
	WorkDir string
	// Printer receives progress and diagnostics.
	Printer *ui.Printer
}

// Result is the outcome for one Go module. A Result is always returned, even
// when the module failed: a failing `go test` still usually writes a partial
// profile, and cover100 reports what it can rather than discarding the module.
type Result struct {
	Dir        string
	Rel        string
	ModulePath string
	Files      []model.FileCoverage
	Warnings   []string
	Command    string
	ExitCode   int
	Output     string
	Err        error
}

// Run executes the module's tests with coverage and parses the profile.
func Run(ctx context.Context, p detect.Project, opts Options, index int) *Result {
	res := &Result{Dir: p.Dir, Rel: p.Rel, ModulePath: p.ModulePath}

	profilePath := filepath.Join(opts.WorkDir, fmt.Sprintf("go-%d.out", index))
	args := []string{"test", "-coverprofile=" + profilePath, "./..."}
	res.Command = "go " + strings.Join(args, " ")

	if opts.Printer != nil {
		opts.Printer.Debugf("%s: running %s", p.Rel, res.Command)
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = p.Dir
	output, err := cmd.CombinedOutput()
	res.Output = truncate(string(output), maxOutputBytes)
	if err != nil {
		res.Err = err
		res.ExitCode = exitCode(err)
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: %s reported test failures (exit %d); parsing whatever coverage was written",
			p.Rel, res.Command, res.ExitCode))
	}

	f, openErr := os.Open(profilePath)
	if openErr != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: no coverage profile was written (%v); the module contributes nothing to the report",
			p.Rel, openErr))
		return res
	}
	defer func() { _ = f.Close() }()

	byFile, parseErr := ParseProfile(f)
	if parseErr != nil {
		res.Err = parseErr
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: parsing coverage profile: %v", p.Rel, parseErr))
		return res
	}
	if len(byFile) == 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: the coverage profile is empty; `go test` writes no profile entries for packages without test files",
			p.Rel))
		return res
	}

	skipped := 0
	for profilePath, lines := range byFile {
		rel, ok := ResolveProfilePath(profilePath, p.ModulePath)
		if !ok {
			skipped++
			continue
		}
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		scanRel, ok := pathutil.RelTo(opts.Root, filepath.Join(p.Dir, filepath.FromSlash(rel)))
		if !ok {
			skipped++
			continue
		}
		res.Files = append(res.Files, model.FileCoverage{
			Path:     scanRel,
			Language: model.LangGo,
			Lines:    metricOf(lines),
			// Go's cover profiles carry statement blocks only. There is no
			// function table to read, so function coverage is reported as
			// unmeasured rather than guessed at.
			Functions:          model.Metric{},
			FunctionsAvailable: false,
		})
	}
	if skipped > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: ignored %d coverage entries outside the module directory", p.Rel, skipped))
	}

	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res
}

// ParseProfile reads a Go cover profile and returns, per file path as written
// in the profile, the hit count of every source line the profile references.
//
// Blocks are expanded across their line range, and a line covered by several
// blocks keeps the highest count — with `-covermode=count` overlapping blocks
// are possible, and a line is covered when any statement on it executed.
func ParseProfile(r io.Reader) (map[string]map[int]int, error) {
	files := make(map[string]map[int]int)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		m := profileRecord.FindStringSubmatch(line)
		if m == nil {
			// Tolerate unrecognised lines rather than failing the module: a
			// profile that is otherwise readable is still worth reporting.
			continue
		}
		startLine, endLine, count := atoi(m[2]), atoi(m[4]), atoi(m[7])
		if endLine < startLine {
			endLine = startLine
		}
		lines := files[m[1]]
		if lines == nil {
			lines = make(map[int]int)
			files[m[1]] = lines
		}
		for l := startLine; l <= endLine; l++ {
			// The comma-ok form matters: a block with count 0 must still record
			// its lines, otherwise uncovered lines vanish from the denominator
			// and every file reports 100% coverage.
			if prev, ok := lines[l]; !ok || count > prev {
				lines[l] = count
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

// ResolveProfilePath converts a path as written in a cover profile into a
// slash-separated path relative to the module directory.
//
// Profile paths are import paths, so the module path prefix is stripped. ok is
// false for entries that do not belong to this module, which happens when a
// profile is produced with -coverpkg or when Go reports a dependency.
func ResolveProfilePath(profilePath, modulePath string) (string, bool) {
	if modulePath == "" {
		return "", false
	}
	switch {
	case profilePath == modulePath:
		return ".", true
	case strings.HasPrefix(profilePath, modulePath+"/"):
		return profilePath[len(modulePath)+1:], true
	default:
		return "", false
	}
}

// metricOf converts a line hit map into a covered/total pair.
func metricOf(lines map[int]int) model.Metric {
	total, covered := len(lines), 0
	for _, hits := range lines {
		if hits > 0 {
			covered++
		}
	}
	return model.Metric{Covered: covered, Total: total}
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
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
