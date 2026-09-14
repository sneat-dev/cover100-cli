package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strongo/logus"
)

func TestPercent(t *testing.T) {
	tests := []struct {
		covered, total int
		want           string
	}{
		{1, 3, "33.3%"},
		{2, 2, "100.0%"},
		{0, 5, "0.0%"},
		// An unmeasured dimension must not be rendered as a real 0%.
		{0, 0, "n/a"},
		{0, -1, "n/a"},
	}
	for _, tc := range tests {
		if got := Percent(tc.covered, tc.total); got != tc.want {
			t.Errorf("Percent(%d, %d) = %q, want %q", tc.covered, tc.total, got, tc.want)
		}
	}
}

func TestNumber(t *testing.T) {
	tests := map[int]string{
		0:       "0",
		42:      "42",
		999:     "999",
		1000:    "1,000",
		12345:   "12,345",
		1234567: "1,234,567",
		-4321:   "-4,321",
	}
	for in, want := range tests {
		if got := Number(in); got != want {
			t.Errorf("Number(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDuration(t *testing.T) {
	tests := map[time.Duration]string{
		250 * time.Millisecond:  "250ms",
		1500 * time.Millisecond: "1.5s",
		90 * time.Second:        "1m 30s",
	}
	for in, want := range tests {
		if got := Duration(in); got != want {
			t.Errorf("Duration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTTY_NonFileWriter(t *testing.T) {
	if IsTTY(&bytes.Buffer{}) {
		t.Error("IsTTY(buffer) = true, want false")
	}
}

// plainPrinter returns a Printer writing to buffers with colour disabled, the
// shape used when output is piped.
func plainPrinter(verbose bool) (*Printer, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	return New(out, errOut, verbose), out, errOut
}

func TestPrinter_PlainOutputUsesFleetPrefixes(t *testing.T) {
	p, out, errOut := plainPrinter(false)

	p.Success("done %d", 1)
	p.Info("working")
	p.Bold("Header")
	p.Warn("careful")
	p.Error("broken")

	if got := out.String(); !strings.Contains(got, "ok: done 1") ||
		!strings.Contains(got, "info: working") || !strings.Contains(got, "Header") {
		t.Errorf("stdout = %q", got)
	}
	if got := errOut.String(); !strings.Contains(got, "warn: careful") || !strings.Contains(got, "error: broken") {
		t.Errorf("stderr = %q", got)
	}
	if strings.Contains(out.String(), "\x1b[") || strings.Contains(errOut.String(), "\x1b[") {
		t.Error("colour escapes must not appear when the destination is not a terminal")
	}
}

func TestPrinter_ColourOutputUsesGlyphs(t *testing.T) {
	p, out, errOut := plainPrinter(false)
	p.color = true

	p.Success("done")
	p.Info("working")
	p.Warn("careful")
	p.Error("broken")

	for _, want := range []string{"\u2713", "\x1b[32m"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
	}
	for _, want := range []string{"\x1b[33m", "\x1b[31m\u2717"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr = %q, want %q", errOut.String(), want)
		}
	}
}

func TestPrinter_DebugRespectsVerbosity(t *testing.T) {
	quiet, _, quietErr := plainPrinter(false)
	quiet.Debugf("hidden %d", 1)
	if quietErr.Len() != 0 {
		t.Errorf("debug output leaked without --verbose: %q", quietErr.String())
	}
	if quiet.Verbose() {
		t.Error("Verbose() = true, want false")
	}

	loud, _, loudErr := plainPrinter(true)
	loud.Debugf("shown %d", 1)
	if !strings.Contains(loudErr.String(), "shown 1") {
		t.Errorf("stderr = %q, want the debug line", loudErr.String())
	}
	if !loud.Verbose() {
		t.Error("Verbose() = true, want false")
	}
}

func TestPrinter_BlockIndentsAndTrimsTrailingNewlines(t *testing.T) {
	p, _, errOut := plainPrinter(false)
	p.Block("first\nsecond\n\n")

	got := errOut.String()
	if !strings.Contains(got, "    first\n    second\n") {
		t.Errorf("Block() = %q", got)
	}
	if strings.HasSuffix(got, "\n\n\n") {
		t.Errorf("Block() left trailing blank lines: %q", got)
	}

	errOut.Reset()
	p.Block("")
	if errOut.Len() != 0 {
		t.Errorf("Block(\"\") wrote %q, want nothing", errOut.String())
	}
}

func TestPrinter_TableAlignsColumns(t *testing.T) {
	p, out, _ := plainPrinter(false)
	p.Table([]string{"repo", "lines"}, [][]string{{"web", "21/40"}, {"api", "6/22"}})

	got := out.String()
	for _, want := range []string{"repo", "lines", "web", "21/40", "api", "6/22"} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q: %q", want, got)
		}
	}
}

func TestPrinter_DetailAndBlank(t *testing.T) {
	p, out, _ := plainPrinter(false)
	p.Detail("note %s", "here")
	p.Blank()
	if got := out.String(); !strings.Contains(got, "note here") || !strings.HasSuffix(got, "\n\n") {
		t.Errorf("output = %q", got)
	}
}

func TestLogHandler_FiltersBySeverity(t *testing.T) {
	p, _, errOut := plainPrinter(false)
	h := NewLogHandler(p)

	if err := h.Log(context.Background(), logus.LogEntry{
		Severity: logus.SeverityDebug, MessageFormat: "debug detail",
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Log(context.Background(), logus.LogEntry{
		Severity: logus.SeverityWarning, MessageFormat: "watch out",
	}); err != nil {
		t.Fatal(err)
	}

	got := errOut.String()
	if strings.Contains(got, "debug detail") {
		t.Errorf("debug entry leaked without --verbose: %q", got)
	}
	if !strings.Contains(got, "watch out") {
		t.Errorf("warning entry missing: %q", got)
	}
}

func TestLogHandler_FormatsComponentArgsAndTraceID(t *testing.T) {
	p, _, errOut := plainPrinter(true)
	h := NewLogHandler(p)

	ctx := logus.WithTraceID(context.Background(), "trace-42")
	if err := h.Log(ctx, logus.LogEntry{
		Severity:      logus.SeverityInfo,
		Component:     "selfupdate",
		MessageFormat: "checked %s",
		MessageArgs:   []any{"v1.2.3"},
	}); err != nil {
		t.Fatal(err)
	}

	got := errOut.String()
	for _, want := range []string{"[trace-42]", "selfupdate:", "checked v1.2.3"} {
		if !strings.Contains(got, want) {
			t.Errorf("log output %q missing %q", got, want)
		}
	}
}

func TestLogHandler_NilPrinterIsSafe(t *testing.T) {
	h := LogHandler{}
	if err := h.Log(context.Background(), logus.LogEntry{MessageFormat: "x"}); err != nil {
		t.Fatalf("Log() = %v, want nil", err)
	}
}
