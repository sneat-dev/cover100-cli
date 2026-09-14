package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	cover100 "github.com/sneat-dev/cover100-cli"
)

func TestRun_HandsErrorsToFatal(t *testing.T) {
	want := errors.New("boom")
	var got error
	fatalCalled := false

	run([]string{"cover100"}, func([]string, fs.FS) error { return want }, func(err error) {
		fatalCalled = true
		got = err
	})

	if !fatalCalled {
		t.Fatal("Fatal was not called for a failing run")
	}
	if !errors.Is(got, want) {
		t.Errorf("Fatal received %v, want %v", got, want)
	}
}

func TestRun_DoesNotCallFatalOnSuccess(t *testing.T) {
	fatalCalled := false

	run([]string{"cover100"}, func([]string, fs.FS) error { return nil }, func(error) {
		fatalCalled = true
	})

	if fatalCalled {
		t.Error("Fatal must not be called when the run succeeds")
	}
}

func TestRun_PassesArgsAndEmbeddedAssetsToTheCLI(t *testing.T) {
	var gotArgs []string
	var gotAssets fs.FS

	args := []string{"cover100", ".", "--no-serve"}
	run(args, func(a []string, assets fs.FS) error {
		gotArgs, gotAssets = a, assets
		return nil
	}, func(error) { t.Error("unexpected Fatal call") })

	if len(gotArgs) != len(args) {
		t.Fatalf("args = %v, want %v", gotArgs, args)
	}
	for i := range args {
		if gotArgs[i] != args[i] {
			t.Errorf("args[%d] = %q, want %q", i, gotArgs[i], args[i])
		}
	}
	if gotAssets == nil {
		t.Fatal("assets = nil, want the embedded viewer filesystem")
	}
	// The embedded viewer must actually contain the page the CLI serves and
	// inlines; a broken embed directive would ship a binary with no UI.
	if _, err := fs.Stat(gotAssets, "public/index.html"); err != nil {
		t.Errorf("embedded assets are missing public/index.html: %v", err)
	}
}

func TestRun_EmbeddedAssetsContainTheViewer(t *testing.T) {
	// A broken embed directive would ship a binary with no UI, and the failure
	// would only surface when a user opened the report.
	for _, name := range []string{"public/index.html", "public/app.js", "public/styles.css"} {
		if _, err := fs.Stat(cover100.PublicFS, name); err != nil {
			t.Errorf("embedded assets are missing %s: %v", name, err)
		}
	}
}

// TestMain_VersionFlagReturnsWithoutExiting drives the real main() entry point.
// It is the only way to cover the wiring that lives in main itself, and it is
// safe because a successful run never reaches cli.Fatal's os.Exit.
func TestMain_VersionFlagReturnsWithoutExiting(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"cover100", "--version"}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		main()
	}()

	select {
	case <-returned:
	case <-time.After(30 * time.Second):
		t.Fatal("main() did not return for --version; it should never block or exit")
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Error("main() printed no version, so the flag never reached the CLI")
	}
}
