package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRelTo(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name   string
		abs    string
		want   string
		wantOK bool
	}{
		{"inside", filepath.Join(root, "src", "a.ts"), "src/a.ts", true},
		{"root itself", root, ".", true},
		{"outside", filepath.Join(root, "..", "elsewhere", "a.ts"), "", false},
		{"sibling prefix", root + "-other/a.ts", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RelTo(root, tc.abs)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("RelTo(%q, %q) = (%q, %v), want (%q, %v)", root, tc.abs, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestRelTo_ResolvesSymlinkedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "src", "a.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// The user asked for the scan root through the link; the coverage tool
	// reported the resolved path. Both name the same file, so it must resolve.
	got, ok := RelTo(link, filepath.Join(real, "src", "a.ts"))
	if !ok {
		t.Fatalf("RelTo(%q, %q) failed to match a symlinked root", link, filepath.Join(real, "src", "a.ts"))
	}
	if got != "src/a.ts" {
		t.Errorf("RelTo() = %q, want src/a.ts", got)
	}
}

func TestRelTo_StillRejectsGenuinelyOutsidePaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, ok := RelTo(root, filepath.Join(outside, "a.ts")); ok {
		t.Error("a path in an unrelated directory must not resolve, even after symlink resolution")
	}
}
