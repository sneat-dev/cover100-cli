package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRelTo_RejectsCanonicalOutsidePathWithoutSymlinkResolution covers the
// case where both paths are already fully resolved: EvalSymlinks changes
// nothing, so the "outside root" verdict must be final rather than being
// retried against the same values. A path that is genuinely outside the root
// must never be reported under a "../" path.
func TestRelTo_RejectsCanonicalOutsidePathWithoutSymlinkResolution(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(root): %v", err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(outside): %v", err)
	}
	abs := filepath.Join(outside, "a.ts")
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := RelTo(root, abs)
	if ok || got != "" {
		t.Errorf("RelTo(%q, %q) = (%q, %v), want (\"\", false): an already-resolved path outside the root must be rejected",
			root, abs, got, ok)
	}
}

// TestRelToRejectsMixedRelativeAndAbsolutePaths drives relTo's filepath.Rel
// error branch directly: Rel cannot express an absolute target relative to a
// relative root, and the helper must report that as "outside root" instead of
// returning an error string as if it were a path.
func TestRelToRejectsMixedRelativeAndAbsolutePaths(t *testing.T) {
	got, ok := relTo("relative/dir", "/absolute/path")
	if ok || got != "" {
		t.Errorf("relTo(\"relative/dir\", \"/absolute/path\") = (%q, %v), want (\"\", false): mismatched relative/absolute paths cannot be related",
			got, ok)
	}

	// Sanity check that the same helper still resolves a genuine relative
	// child, so the rejection above is about the mismatch and not a blanket
	// failure.
	if got, ok := relTo("relative/dir", "relative/dir/child"); !ok || got != "child" {
		t.Errorf("relTo of a relative child = (%q, %v), want (\"child\", true)", got, ok)
	}
}
