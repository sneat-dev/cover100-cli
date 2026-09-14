package main

import (
	"errors"
	"io/fs"
	"testing"

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
