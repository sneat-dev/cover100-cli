package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/strongo/logus"

	"github.com/sneat-dev/cover100-cli/internal/aggregate"
	"github.com/sneat-dev/cover100-cli/internal/detect"
	"github.com/sneat-dev/cover100-cli/internal/gocov"
	"github.com/sneat-dev/cover100-cli/internal/model"
	"github.com/sneat-dev/cover100-cli/internal/serve"
	"github.com/sneat-dev/cover100-cli/internal/tscov"
	"github.com/sneat-dev/cover100-cli/internal/ui"
	"github.com/sneat-dev/cover100-cli/internal/view"
	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

const (
	defaultOut    = ".cover100/coverage.json"
	defaultPort   = 5173
	defaultMetric = "lines"
	defaultMode   = "percent"
	// defaultTimeout bounds one project's coverage command. It is generous
	// because a cold build in a large repository is legitimately slow, while an
	// unbounded wait would hang a CI job forever.
	defaultTimeout = 15 * time.Minute
	// standaloneName is the self-contained viewer written beside the report.
	standaloneName = "view.html"
	// maxParallelProjects caps concurrent coverage commands so a monorepo does
	// not fork one test runner per package.
	maxParallelProjects = 4
	// failureTailLines is how much of a failed command's output is shown. Its
	// last lines are the summary — "FAIL pkg [build failed]", the failing test
	// names — which is what a reader needs in order to act; the full transcript
	// is noise.
	failureTailLines = 12
)

// logusOnce guards handler registration: logus panics on a duplicate handler,
// and Run may be called more than once in tests.
var logusOnce sync.Once

// absPath and marshalReport are test seams.
//
// Both guard failures that are real but cannot be provoked from the
// environment here: resolving a relative --out needs a readable working
// directory (which macOS still reports after its directory is deleted), and
// model.Report is JSON-safe by construction, so its encoder only fails for a
// reason a future field could introduce. Routing them through a seam is what
// makes those guards provable instead of permanently unexercised.
var (
	absPath       = filepath.Abs
	marshalReport = json.MarshalIndent
)

// collectOptions mirrors the command's flags.
type collectOptions struct {
	path    string
	out     string
	open    bool
	noOpen  bool
	lang    string
	metric  string
	mode    string
	port    int
	keep    bool
	noServe bool
	file    bool
	timeout time.Duration
	format  string
	verbose bool
}

func (o *collectOptions) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&o.out, "out", defaultOut, "path to write the coverage report JSON")
	f.BoolVar(&o.open, "open", true, "open the treemap in the default browser")
	f.BoolVar(&o.noOpen, "no-open", false, "do not open a browser (overrides --open)")
	f.StringVar(&o.lang, "lang", "all", "restrict languages: go|ts|js|all")
	f.StringVar(&o.metric, "metric", defaultMetric,
		"metric the treemap opens with: lines|functions|files|packages|repositories")
	f.StringVar(&o.mode, "mode", defaultMode, "colour mode the treemap opens with: percent|count")
	f.IntVar(&o.port, "port", defaultPort, "port for the local static server")
	f.BoolVar(&o.keep, "keep", false, "keep intermediate coverage artefacts under the output directory")
	f.BoolVar(&o.noServe, "no-serve", false, "write the report and exit without starting the static server")
	f.BoolVar(&o.file, "file", false, "open the self-contained report over file:// instead of serving it")
	f.DurationVar(&o.timeout, "timeout", defaultTimeout, "timeout for each project's coverage command")
	f.StringVar(&o.format, "format", "text", "output format: text|json")
	f.BoolVar(&o.verbose, "verbose", false, "verbose diagnostic logging")
}

// wantsOpen reports whether a browser should be launched.
func (o *collectOptions) wantsOpen() bool { return o.open && !o.noOpen }

func (o *collectOptions) validate() error {
	switch o.lang {
	case "all", "go", "ts", "js":
	default:
		return exitcode.InvalidArgsErrorf("invalid --lang %q: expected go, ts, js or all", o.lang)
	}
	if !contains(model.MetricNames, o.metric) {
		return exitcode.InvalidArgsErrorf("invalid --metric %q: expected one of %s",
			o.metric, strings.Join(model.MetricNames, ", "))
	}
	switch o.mode {
	case "percent", "count":
	default:
		return exitcode.InvalidArgsErrorf("invalid --mode %q: expected percent or count", o.mode)
	}
	switch o.format {
	case "text", "json":
	default:
		return exitcode.InvalidArgsErrorf("invalid --format %q: expected text or json", o.format)
	}
	if o.port < 0 || o.port > 65535 {
		return exitcode.InvalidArgsErrorf("invalid --port %d: expected 0..65535", o.port)
	}
	if o.timeout <= 0 {
		return exitcode.InvalidArgsErrorf("invalid --timeout %s: must be positive", o.timeout)
	}
	return nil
}

// summary is the machine-readable run result emitted by --format=json.
type summary struct {
	Tool       string          `json:"tool"`
	Root       string          `json:"root"`
	Report     string          `json:"report"`
	Standalone string          `json:"standalone,omitempty"`
	URL        string          `json:"url,omitempty"`
	Languages  []string        `json:"languages"`
	Projects   int             `json:"projects"`
	Files      int             `json:"files"`
	Metrics    []metricSummary `json:"metrics"`
	Warnings   []string        `json:"warnings,omitempty"`
	DurationMS int64           `json:"durationMs"`
}

type metricSummary struct {
	Metric  string  `json:"metric"`
	Covered int     `json:"covered"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// runCollect is the root command's RunE: collect, write, and show.
func runCollect(ctx context.Context, cmd *cobra.Command, args []string, assets fs.FS, opts *collectOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}

	// stdout carries exactly one JSON document under --format=json, so progress
	// moves to stderr and the command stays scriptable.
	progressOut := cmd.OutOrStdout()
	if opts.format == "json" {
		progressOut = cmd.ErrOrStderr()
	}
	printer := ui.New(progressOut, cmd.ErrOrStderr(), opts.verbose)
	logusOnce.Do(func() { logus.AddLogEntryHandler(ui.NewLogHandler(printer)) })

	// The embedded filesystem is rooted at the repository, so the viewer's
	// assets are addressed relative to public/.
	public, err := fs.Sub(assets, "public")
	if err != nil {
		return exitcode.UnexpectedErrorCause("locating the bundled viewer assets", err)
	}

	root, err := resolveArg(args)
	if err != nil {
		return exitcode.UnexpectedErrorCause("resolving the scan path", err)
	}

	started := time.Now()
	if opts.format == "text" {
		printer.Bold("%s %s", binaryName, buildInfo.Short())
		printer.Info("scanning %s", root)
		printer.Blank()
	}

	scan, err := detect.Scan(root)
	if err != nil {
		return exitcode.NotFoundErrorCause(fmt.Sprintf("cannot scan %s: %v", root, err), err)
	}
	goProjects, nodeProjects := selectProjects(scan, opts.lang)
	if len(goProjects) == 0 && len(nodeProjects) == 0 {
		return exitcode.NotFoundErrorf(
			"no Go module (go.mod) or Node package (package.json with a test script) found under %s", root)
	}
	if opts.format == "text" {
		printer.Bold("Detecting projects")
		if n := len(goProjects); n > 0 {
			printer.Success("%s", plural(n, "Go module", "Go modules"))
		}
		if n := len(nodeProjects); n > 0 {
			printer.Success("%s", plural(n, "Node package with a test script",
				"Node packages with a test script"))
		}
		printer.Blank()
	}

	outPath, err := absPath(opts.out)
	if err != nil {
		return exitcode.UnexpectedErrorCause("resolving --out", err)
	}
	workDir := filepath.Dir(outPath)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("creating %s", workDir), err)
	}

	if opts.format == "text" {
		printer.Bold("Collecting coverage")
	}
	goResults, nodeResults := collectAll(ctx, printer, root, workDir, goProjects, nodeProjects, opts.timeout)

	// A cancelled context means the user interrupted us: report that rather
	// than writing a report built from half-finished runs.
	if ctx.Err() != nil {
		return exitcode.New(130, "interrupted")
	}

	projects, warnings := assemble(goResults, nodeResults, opts.lang)
	report := aggregate.Build(aggregate.Input{
		Root:     root,
		Projects: projects,
		Warnings: warnings,
		Tool:     binaryName + " " + buildInfo.Short(),
	})

	data, err := marshalReport(report, "", "  ")
	if err != nil {
		return exitcode.UnexpectedErrorCause("encoding the report", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("writing %s", outPath), err)
	}

	standalonePath := filepath.Join(workDir, standaloneName)
	standaloneReady := false
	if standalone, buildErr := view.Standalone(public, data); buildErr != nil {
		// The served page still works, so this is a warning with a clear cause
		// rather than a failed run.
		printer.Warn("could not build the standalone report: %v", buildErr)
	} else if writeErr := os.WriteFile(standalonePath, standalone, 0o644); writeErr != nil {
		printer.Warn("could not write %s: %v", standalonePath, writeErr)
	} else {
		standaloneReady = true
	}

	cleanupIntermediates(printer, goResults, nodeResults, workDir, opts.keep)

	for _, w := range report.Warnings {
		printer.Warn("%s", w)
	}

	if opts.format == "text" {
		printer.Blank()
		printer.Bold("Report")
		printMetrics(printer, report)
		printer.Blank()
		printer.Success("wrote %s (%s)", outPath, humanBytes(len(data)))
		if standaloneReady {
			printer.Success("wrote %s", standalonePath)
		}
	}

	sum := buildSummary(report, outPath, standalonePath, time.Since(started), standaloneReady, len(projects))

	if opts.noServe {
		if opts.format == "json" {
			return encodeSummary(cmd, sum)
		}
		return nil
	}

	if opts.file {
		fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(standalonePath)}).String()
		sum.URL = fileURL
		if opts.format == "text" {
			printer.Blank()
			printer.Info("opening %s", fileURL)
		}
		if opts.format == "json" {
			if err := encodeSummary(cmd, sum); err != nil {
				return err
			}
		}
		if opts.wantsOpen() {
			if err := serve.OpenBrowser(fileURL); err != nil {
				printer.Warn("%v", err)
			}
		}
		return nil
	}

	return serveReport(ctx, cmd, printer, opts, public, outPath, sum)
}

// reportServer is the part of *serve.Server the CLI drives.
//
// It is an interface rather than the concrete type so a test can substitute a
// server whose Serve fails: the real one only returns an error on a listener
// fault that cannot be provoked portably, and "serving the report failed" is a
// path worth proving rather than assuming.
type reportServer interface {
	PageURL(metric, mode string) string
	Serve() error
	Close() error
}

// startReportServer is the seam over serve.Start.
var startReportServer = func(opts serve.Options) (reportServer, error) {
	return serve.Start(opts)
}

// serveReport starts the static server, opens the page and blocks until the
// context is cancelled.
func serveReport(
	ctx context.Context,
	cmd *cobra.Command,
	printer *ui.Printer,
	opts *collectOptions,
	assets fs.FS,
	outPath string,
	sum summary,
) error {
	srv, err := startReportServer(serve.Options{
		Assets:    assets,
		DataPath:  outPath,
		DataRoute: "/coverage.json",
		Port:      opts.port,
		Printer:   printer,
	})
	if err != nil {
		return exitcode.UnexpectedErrorCause("starting the static server", err)
	}
	defer func() { _ = srv.Close() }()

	pageURL := srv.PageURL(opts.metric, opts.mode)
	sum.URL = pageURL
	if opts.format == "text" {
		printer.Blank()
		printer.Info("serving %s", pageURL)
		printer.Info("press Ctrl+C to stop")
	} else {
		if err := encodeSummary(cmd, sum); err != nil {
			return err
		}
	}
	if opts.wantsOpen() {
		if err := serve.OpenBrowser(pageURL); err != nil {
			printer.Warn("%v", err)
		}
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve() }()

	select {
	case <-ctx.Done():
		if opts.format == "text" {
			printer.Blank()
			printer.Info("shutting down")
		}
		return exitcode.New(130, "interrupted")
	case err := <-serveErr:
		if err != nil {
			return exitcode.UnexpectedErrorCause("serving the report", err)
		}
		return nil
	}
}

// encodeSummary writes the machine-readable summary and nothing else.
func encodeSummary(cmd *cobra.Command, sum summary) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(sum); err != nil {
		return exitcode.UnexpectedErrorCause("encoding the summary", err)
	}
	return nil
}

// collectAll runs every project's coverage command with bounded concurrency.
// One project's failure never cancels another: each failure becomes a warning
// on that project's result.
func collectAll(
	ctx context.Context,
	printer *ui.Printer,
	root, workDir string,
	goProjects, nodeProjects []detect.Project,
	timeout time.Duration,
) ([]*gocov.Result, []*tscov.Result) {
	goResults := make([]*gocov.Result, len(goProjects))
	nodeResults := make([]*tscov.Result, len(nodeProjects))

	sem := make(chan struct{}, concurrency())
	var wg sync.WaitGroup

	for i := range goProjects {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			project := goProjects[i]
			projectCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			res := gocov.Run(projectCtx, project, gocov.Options{
				Root: root, WorkDir: workDir, Printer: printer,
			}, i)
			goResults[i] = res
			printer.Success("%s", projectLine("go", project.Rel, res.ModulePath,
				res.Files, time.Since(started)))
			printFailureOutput(printer, project.Rel, res.Err, res.Output)
		}(i)
	}

	for i := range nodeProjects {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			project := nodeProjects[i]
			projectCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			res := tscov.Run(projectCtx, project, tscov.Options{
				Root: root, WorkDir: workDir, Printer: printer,
			}, i)
			nodeResults[i] = res
			// res.Runner is always populated by tscov.Run, including the
			// fallback it applies when a package declares no runner, so there
			// is nothing to second-guess here.
			printer.Success("%s", projectLine("js", project.Rel, res.Runner,
				res.Files, time.Since(started)))
			printFailureOutput(printer, project.Rel, res.Err, res.Output)
		}(i)
	}

	wg.Wait()
	return goResults, nodeResults
}

// printFailureOutput shows why a collection command failed.
//
// A partial report is only diagnosable if the reader can see what went wrong:
// without this the run says "reported test failures" and leaves them to re-run
// the suite by hand to find out which package failed. The transcript is bounded
// to its last lines, which for `go test ./...` and the JS runners is where the
// failure summary lives.
func printFailureOutput(printer *ui.Printer, label string, err error, output string) {
	if err == nil || strings.TrimSpace(output) == "" {
		return
	}
	name := label
	if name == "" {
		name = "."
	}
	printer.Block(fmt.Sprintf("%s failed; last %d lines of its output:\n%s",
		name, failureTailLines, tailLines(output, failureTailLines)))
}

// tailLines returns at most the last n lines of s.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if n <= 0 || len(lines) == 0 {
		return ""
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// projectLine renders one project's outcome as a single line, so concurrent
// projects never interleave mid-line.
func projectLine(kind, rel, detail string, files []model.FileCoverage, elapsed time.Duration) string {
	var lines model.Metric
	for _, f := range files {
		lines = lines.Add(f.Lines)
	}
	label := rel
	if label == "" {
		label = "."
	}
	if detail != "" {
		label += " (" + detail + ")"
	}
	return fmt.Sprintf("%-3s %-42s %6s lines %8s  %5s",
		kind, truncateLabel(label, 42), ui.Percent(lines.Covered, lines.Total),
		fmt.Sprintf("%s/%s", ui.Number(lines.Covered), ui.Number(lines.Total)), ui.Duration(elapsed))
}

// assemble converts collector results into the aggregation input, applying the
// language restriction to individual files.
func assemble(
	goResults []*gocov.Result,
	nodeResults []*tscov.Result,
	lang string,
) ([]aggregate.ProjectFiles, []string) {
	byRepo := make(map[string]*aggregate.ProjectFiles)
	var order []string
	var warnings []string

	add := func(rel string, files []model.FileCoverage) {
		if len(files) == 0 {
			return
		}
		group := byRepo[rel]
		if group == nil {
			group = &aggregate.ProjectFiles{Rel: rel}
			byRepo[rel] = group
			order = append(order, rel)
		}
		group.Files = append(group.Files, files...)
	}

	for _, res := range goResults {
		if res == nil {
			continue
		}
		warnings = append(warnings, res.Warnings...)
		add(res.Rel, res.Files)
	}
	for _, res := range nodeResults {
		if res == nil {
			continue
		}
		warnings = append(warnings, res.Warnings...)
		files := res.Files
		if lang == "ts" || lang == "js" {
			filtered := make([]model.FileCoverage, 0, len(files))
			for _, f := range files {
				if (lang == "ts" && f.Language == model.LangTypeScript) ||
					(lang == "js" && f.Language == model.LangJavaScript) {
					filtered = append(filtered, f)
				}
			}
			if len(filtered) == 0 && len(files) > 0 {
				warnings = append(warnings, fmt.Sprintf(
					"%s: no %s coverage found; the package is excluded from this report", res.Rel, lang))
			}
			files = filtered
		}
		add(res.Rel, files)
	}

	out := make([]aggregate.ProjectFiles, 0, len(order))
	for _, rel := range order {
		out = append(out, *byRepo[rel])
	}
	return out, warnings
}

// cleanupIntermediates deletes the profiles and coverage directories cover100
// created. It removes only paths this run tracked, so the report and the
// standalone viewer — which live in the same directory by default — survive.
func cleanupIntermediates(
	printer *ui.Printer,
	goResults []*gocov.Result,
	nodeResults []*tscov.Result,
	workDir string,
	keep bool,
) {
	if keep {
		printer.Info("keeping intermediate coverage artefacts in %s", workDir)
		return
	}
	for i := range goResults {
		path := filepath.Join(workDir, fmt.Sprintf("go-%d.out", i))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			printer.Debugf("removing %s: %v", path, err)
		}
	}
	for i := range nodeResults {
		path := filepath.Join(workDir, fmt.Sprintf("ts-%d", i))
		if err := os.RemoveAll(path); err != nil {
			printer.Debugf("removing %s: %v", path, err)
		}
	}
}

// printMetrics renders the whole-tree summary table.
func printMetrics(printer *ui.Printer, report *model.Report) {
	headers := []string{"repository", "language", "lines", "functions", "files"}
	rows := make([][]string, 0, len(report.Tree.Children)+1)
	for _, repo := range report.Tree.Children {
		rows = append(rows, metricRow(repo.Name, languageOf(repo), repo))
	}
	rows = append(rows, metricRow("overall", "", report.Tree))
	printer.Table(headers, rows)
}

func metricRow(name, language string, node *model.Node) []string {
	return []string{
		name,
		language,
		metricCell(node.Lines),
		metricCell(node.Functions),
		metricCell(node.Files),
	}
}

func metricCell(m model.Metric) string {
	if m.Total == 0 {
		return "not measured"
	}
	return fmt.Sprintf("%s/%s %s", ui.Number(m.Covered), ui.Number(m.Total), ui.Percent(m.Covered, m.Total))
}

func languageOf(node *model.Node) string {
	if node.Language == nil {
		return ""
	}
	return *node.Language
}

// buildSummary assembles the --format=json payload.
func buildSummary(
	report *model.Report,
	reportPath, standalonePath string,
	elapsed time.Duration,
	standaloneReady bool,
	projects int,
) summary {
	sum := summary{
		Tool:       report.Tool,
		Root:       report.Root,
		Report:     reportPath,
		Languages:  report.Languages,
		Projects:   projects,
		Warnings:   report.Warnings,
		DurationMS: elapsed.Milliseconds(),
	}
	if standaloneReady {
		sum.Standalone = standalonePath
	}
	sum.Files = report.Tree.Files.Total
	sum.Metrics = []metricSummary{
		toMetricSummary("lines", report.Tree.Lines),
		toMetricSummary("functions", report.Tree.Functions),
		toMetricSummary("files", report.Tree.Files),
		toMetricSummary("packages", report.Tree.Packages),
		toMetricSummary("repositories", report.Tree.Repositories),
	}
	return sum
}

func toMetricSummary(name string, m model.Metric) metricSummary {
	out := metricSummary{Metric: name, Covered: m.Covered, Total: m.Total}
	if pct, ok := m.Percent(); ok {
		out.Percent = pct
	}
	return out
}

// selectProjects applies the --lang restriction at project granularity.
func selectProjects(scan *detect.Result, lang string) (goProjects, nodeProjects []detect.Project) {
	switch lang {
	case "go":
		return scan.GoModules(), nil
	case "ts", "js":
		return nil, scan.NodePackages()
	default:
		return scan.GoModules(), scan.NodePackages()
	}
}

// concurrency returns how many coverage commands may run at once.
func concurrency() int { return concurrencyFor(runtime.NumCPU()) }

// concurrencyFor bounds the worker pool for a machine with cpus cores: at most
// maxParallelProjects, never fewer than two. A monorepo with many packages
// should not fork one test runner per package.
func concurrencyFor(cpus int) int {
	n := cpus
	if n > maxParallelProjects {
		n = maxParallelProjects
	}
	if n < 2 {
		n = 2
	}
	return n
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func truncateLabel(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return "..." + s[len(s)-(max-3):]
}

func humanBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/unit)
}
