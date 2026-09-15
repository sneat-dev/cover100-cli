package cli

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/ui"
)

func TestTailLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"fewer lines than asked for", "one\ntwo", 5, "one\ntwo"},
		{"exactly the budget", "one\ntwo\nthree", 3, "one\ntwo\nthree"},
		{"keeps the tail", "one\ntwo\nthree\nfour", 2, "three\nfour"},
		{"trailing newline is not an empty line", "one\ntwo\n", 5, "one\ntwo"},
		{"empty input", "", 5, ""},
		{"a non-positive budget yields nothing", "one\ntwo", 0, ""},
		{"a single line", "only", 3, "only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tailLines(tc.in, tc.n); got != tc.want {
				t.Errorf("tailLines(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestPrintFailureOutput(t *testing.T) {
	t.Run("shows the failure summary of a failed command", func(t *testing.T) {
		var stderr strings.Builder
		printer := ui.New(io.Discard, &stderr, false)
		output := "ok  \texample.com/a\t0.1s\n" +
			"--- FAIL: TestThing (0.2s)\n" +
			"FAIL\nexit status 1\nFAIL\texample.com/b\t0.3s\n"

		printFailureOutput(printer, "wb", errors.New("exit status 1"), output)

		got := stderr.String()
		// A part-covered report is only diagnosable if the run says which
		// package failed, not merely that something did.
		for _, want := range []string{"wb failed", "FAIL\texample.com/b", "exit status 1"} {
			if !strings.Contains(got, want) {
				t.Errorf("stderr = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("a successful command says nothing", func(t *testing.T) {
		var stderr strings.Builder
		printFailureOutput(ui.New(io.Discard, &stderr, false), "wb", nil, "some output")
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing when the command succeeded", stderr.String())
		}
	})

	t.Run("a silent failure says nothing", func(t *testing.T) {
		var stderr strings.Builder
		printFailureOutput(ui.New(io.Discard, &stderr, false), "wb", errors.New("exit status 1"), "   \n")
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing when the command captured no output", stderr.String())
		}
	})

	t.Run("an unnamed project is labelled as the root", func(t *testing.T) {
		var stderr strings.Builder
		printFailureOutput(ui.New(io.Discard, &stderr, false), "", errors.New("boom"), "FAIL\tsomething\n")
		// Block indents its lines, so the label is checked for presence rather
		// than as a prefix.
		if !strings.Contains(stderr.String(), ". failed;") {
			t.Errorf("stderr = %q, want the root project labelled \".\"", stderr.String())
		}
	})

	t.Run("a long transcript is bounded to its tail", func(t *testing.T) {
		var stderr strings.Builder
		long := strings.Repeat("noise line\n", 500) + "FAIL\tthe-real-cause\n"
		printFailureOutput(ui.New(io.Discard, &stderr, false), "wb", errors.New("exit status 1"), long)

		got := stderr.String()
		if !strings.Contains(got, "the-real-cause") {
			t.Error("the tail must include the end of the transcript, where the summary is")
		}
		if n := strings.Count(got, "noise line"); n > failureTailLines {
			t.Errorf("showed %d noise lines, want at most %d", n, failureTailLines)
		}
	})
}
