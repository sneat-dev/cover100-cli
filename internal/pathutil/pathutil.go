// Package pathutil holds the small path helpers shared by cover100's
// collectors.
package pathutil

import (
	"path/filepath"
	"strings"
)

// RelTo returns abs expressed relative to root using forward slashes, which is
// the form every node id and report path uses.
//
// ok is false when abs lies outside root: a coverage report may legitimately
// mention files the scan did not cover (a monorepo sibling, a linked package),
// and those must be dropped rather than reported under a "../" path.
//
// Coverage tools report resolved paths, while the scan root is whatever the
// user typed, so the two can name the same directory through different symlink
// chains — on macOS `/tmp` is a symlink to `/private/tmp`, and a checkout can
// be reached through a linked workspace directory. When the literal comparison
// fails, both paths are resolved and compared again.
func RelTo(root, abs string) (string, bool) {
	if rel, ok := relTo(root, abs); ok {
		return rel, ok
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	resolvedAbs, absErr := filepath.EvalSymlinks(abs)
	if rootErr != nil || absErr != nil {
		return "", false
	}
	if resolvedRoot == root && resolvedAbs == abs {
		// Nothing was resolved away: the path is genuinely outside root.
		return "", false
	}
	return relTo(resolvedRoot, resolvedAbs)
}

func relTo(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	// filepath.Rel returns "." for identical paths and never "": an empty
	// result comes only with an error, which is handled above.
	return rel, true
}
