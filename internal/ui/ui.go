// Package ui provides cover100's terminal styling and wire-up to
// github.com/strongo/logus.
//
// The styling vocabulary — the ✓/i/!/✗ glyphs, the bold headers, the dim/cyan
// helpers, and the plain "ok:"/"info:"/"warn:"/"error:" fallbacks used when
// stdout is not a terminal — follows the fleet's other Go CLIs so cover100
// looks and pipes like the rest of them.
package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/strongo/logus"
)

// Printer writes cover100's progress, summary and diagnostics. It is safe for
// concurrent use: collectors run in parallel and share one Printer.
type Printer struct {
	out     io.Writer
	err     io.Writer
	verbose bool
	color   bool
	mu      sync.Mutex
}

// New returns a Printer writing progress to out and warnings, errors and
// diagnostics to err. Colour is enabled only when out is a terminal and
// NO_COLOR is unset.
func New(out, err io.Writer, verbose bool) *Printer {
	return &Printer{out: out, err: err, verbose: verbose, color: IsTTY(out)}
}

// IsTTY reports whether w is a character device, matching the fleet's other
// CLIs, which gate all styling on exactly this check.
func IsTTY(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Verbose reports whether --verbose was passed.
func (p *Printer) Verbose() bool { return p.verbose }

func (p *Printer) style(code, s string) string {
	if !p.color {
		return s
	}
	return code + s + "\x1b[0m"
}

func (p *Printer) line(w io.Writer, prefix, format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(w, "%s %s\n", prefix, fmt.Sprintf(format, args...))
}

// Bold writes a section header.
func (p *Printer) Bold(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.color {
		_, _ = fmt.Fprintln(p.out, "\x1b[1m"+msg+"\x1b[0m")
		return
	}
	_, _ = fmt.Fprintln(p.out, msg)
}

// Info writes a neutral progress line.
func (p *Printer) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if p.color {
		p.line(p.out, "\x1b[34mi\x1b[0m", "%s", msg)
		return
	}
	p.line(p.out, "info:", "%s", msg)
}

// Success writes a completed step.
func (p *Printer) Success(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if p.color {
		p.line(p.out, "\x1b[32m\u2713\x1b[0m", "%s", msg)
		return
	}
	p.line(p.out, "ok:", "%s", msg)
}

// Warn writes a warning to stderr.
//
// It deliberately does not mirror through logus: the LogHandler below prints
// logus entries to the same stream, and routing a user-facing warning through
// both would report it twice.
func (p *Printer) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if p.color {
		p.line(p.err, "\x1b[33m!\x1b[0m", "%s", msg)
		return
	}
	p.line(p.err, "warn:", "%s", msg)
}

// Error writes a failure to stderr.
func (p *Printer) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if p.color {
		p.line(p.err, "\x1b[31m\u2717\x1b[0m", "%s", msg)
		return
	}
	p.line(p.err, "error:", "%s", msg)
}

// Debugf writes a diagnostic line only when --verbose is set.
func (p *Printer) Debugf(format string, args ...any) {
	if !p.verbose {
		return
	}
	msg := fmt.Sprintf(format, args...)
	p.line(p.err, p.style("\x1b[2m", "debug:"), "%s", msg)
}

// Detail writes an indented continuation line under a progress line.
func (p *Printer) Detail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	p.line(p.out, "  "+p.style("\x1b[2m", "-"), "%s", msg)
}

// Blank writes an empty line.
func (p *Printer) Blank() {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintln(p.out)
}

// Block writes captured command output, indented and dimmed.
func (p *Printer) Block(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	var b strings.Builder
	for _, l := range strings.Split(text, "\n") {
		b.WriteString("    ")
		b.WriteString(p.style("\x1b[2m", l))
		b.WriteString("\n")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprint(p.err, b.String())
}

// Table renders an aligned table with a bold header row.
func (p *Printer) Table(headers []string, rows [][]string) {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	sep := make([]string, len(headers))
	for i, h := range headers {
		sep[i] = strings.Repeat("-", len(h))
	}
	_, _ = fmt.Fprintln(tw, strings.Join(sep, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprint(p.out, b.String())
}

// LogHandler adapts github.com/strongo/logus to this Printer. Register it with
// logus.AddLogEntryHandler so every package that logs through logus lands in
// cover100's output, filtered by verbosity.
type LogHandler struct{ printer *Printer }

// NewLogHandler returns a logus handler backed by p.
func NewLogHandler(p *Printer) LogHandler { return LogHandler{printer: p} }

// Log implements logus.LogEntryHandler.
func (h LogHandler) Log(ctx context.Context, entry logus.LogEntry) error {
	if h.printer == nil {
		return nil
	}
	switch entry.Severity {
	case logus.SeverityDebug, logus.SeverityDefault, logus.SeverityInfo, logus.SeverityNotice:
		if !h.printer.verbose {
			return nil
		}
	}

	msg := entry.MessageFormat
	if len(entry.MessageArgs) > 0 {
		msg = fmt.Sprintf(entry.MessageFormat, entry.MessageArgs...)
	}
	if entry.Component != "" {
		msg = entry.Component + ": " + msg
	}
	if traceID := logus.GetTraceID(ctx); traceID != "" {
		msg = "[" + traceID + "] " + msg
	}
	p := h.printer
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(p.err, "%s %s\n", p.style("\x1b[2m", "log:"), msg)
	return nil
}

// Percent formats a covered/total pair as "xx.x%", or "n/a" when the
// dimension was not measured.
func Percent(covered, total int) string {
	if total <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", float64(covered)/float64(total)*100)
}

// Number formats an integer with comma separators.
func Number(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Duration formats a duration the way the fleet's other CLIs do.
func Duration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	sec := d.Seconds()
	if sec < 60 {
		return fmt.Sprintf("%.1fs", sec)
	}
	min := int(sec) / 60
	return fmt.Sprintf("%dm %.0fs", min, sec-float64(min*60))
}
