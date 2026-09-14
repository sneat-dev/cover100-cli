package cli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/detect"
	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

func TestCollectOptionsValidate(t *testing.T) {
	valid := func() collectOptions {
		return collectOptions{
			lang: "all", metric: "lines", mode: "percent",
			port: 5173, timeout: defaultTimeout, format: "text", out: defaultOut,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*collectOptions)
		wantErr bool
	}{
		{"defaults", func(*collectOptions) {}, false},
		{"lang go", func(o *collectOptions) { o.lang = "go" }, false},
		{"lang ts", func(o *collectOptions) { o.lang = "ts" }, false},
		{"lang js", func(o *collectOptions) { o.lang = "js" }, false},
		{"bad lang", func(o *collectOptions) { o.lang = "rust" }, true},
		{"metric functions", func(o *collectOptions) { o.metric = "functions" }, false},
		{"bad metric", func(o *collectOptions) { o.metric = "statements" }, true},
		{"mode count", func(o *collectOptions) { o.mode = "count" }, false},
		{"bad mode", func(o *collectOptions) { o.mode = "absolute" }, true},
		{"format json", func(o *collectOptions) { o.format = "json" }, false},
		{"bad format", func(o *collectOptions) { o.format = "yaml" }, true},
		{"negative port", func(o *collectOptions) { o.port = -1 }, true},
		{"port too large", func(o *collectOptions) { o.port = 70000 }, true},
		{"zero port is allowed", func(o *collectOptions) { o.port = 0 }, false},
		{"zero timeout", func(o *collectOptions) { o.timeout = 0 }, true},
		{"negative timeout", func(o *collectOptions) { o.timeout = -1 }, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := valid()
			tc.mutate(&opts)

			err := opts.validate()
			if tc.wantErr {
				if err == nil {
					t.Fatal("validate() = nil, want an error")
				}
				var coded interface{ ExitCode() int }
				if !errors.As(err, &coded) || coded.ExitCode() != exitcode.InvalidArgs {
					t.Errorf("validate() error = %v, want exit code %d", err, exitcode.InvalidArgs)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestWantsOpen(t *testing.T) {
	tests := []struct {
		open, noOpen, want bool
	}{
		{true, false, true},
		{true, true, false},
		{false, false, false},
		{false, true, false},
	}
	for _, tc := range tests {
		opts := collectOptions{open: tc.open, noOpen: tc.noOpen}
		if got := opts.wantsOpen(); got != tc.want {
			t.Errorf("open=%v noOpen=%v wantsOpen() = %v, want %v", tc.open, tc.noOpen, got, tc.want)
		}
	}
}

func TestResolveArg(t *testing.T) {
	t.Run("defaults to the working directory", func(t *testing.T) {
		got, err := resolveArg(nil)
		if err != nil {
			t.Fatalf("resolveArg(nil) error = %v", err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("resolveArg(nil) = %q, want an absolute path", got)
		}
	})

	t.Run("resolves a relative argument", func(t *testing.T) {
		got, err := resolveArg([]string{"."})
		if err != nil {
			t.Fatalf("resolveArg() error = %v", err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("resolveArg(\".\") = %q, want an absolute path", got)
		}
	})
}

func TestSelectProjects(t *testing.T) {
	scan := &detect.Result{
		Root: "/work/app",
		Projects: []detect.Project{
			{Dir: "/work/app", Rel: ".", Kind: detect.KindGo, ModulePath: "example.com/app"},
			{Dir: "/work/app/web", Rel: "web", Kind: detect.KindNode,
				PackageName: "web", TestScript: "vitest run", HasTestScript: true, Runner: "vitest"},
		},
	}

	tests := []struct {
		lang     string
		wantGo   int
		wantNode int
	}{
		{"all", 1, 1},
		{"go", 1, 0},
		{"ts", 0, 1},
		{"js", 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			goProjects, nodeProjects := selectProjects(scan, tc.lang)
			if len(goProjects) != tc.wantGo || len(nodeProjects) != tc.wantNode {
				t.Errorf("selectProjects(%q) = (%d go, %d node), want (%d, %d)",
					tc.lang, len(goProjects), len(nodeProjects), tc.wantGo, tc.wantNode)
			}
		})
	}
}

func TestTruncateLabel(t *testing.T) {
	if got := truncateLabel("short", 42); got != "short" {
		t.Errorf("truncateLabel(short) = %q", got)
	}
	got := truncateLabel("a-very-long-repository-name-that-exceeds-the-budget", 20)
	if len(got) != 20 {
		t.Errorf("truncateLabel() length = %d, want 20 (%q)", len(got), got)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int]string{
		512:     "512 B",
		2048:    "2.0 KiB",
		5 << 20: "5.0 MiB",
		3 << 30: "3.0 GiB",
	}
	for in, want := range tests {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "Go module", "Go modules"); got != "1 Go module" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(2, "Go module", "Go modules"); got != "2 Go modules" {
		t.Errorf("plural(2) = %q", got)
	}
}
