package cli

import (
	"os"
	"path/filepath"
)

// getwd is a seam over os.Getwd.
//
// The "no working directory" failure has to be provable on every platform, and
// it cannot be provoked from the environment: after its directory is removed,
// macOS still returns the stale path while Linux fails with ENOENT. Driving it
// through this seam keeps the test honest on both instead of passing only where
// the kernel happens to disagree.
var getwd = os.Getwd

// resolveArg returns the scan root as an absolute path: the sole positional
// argument when given, otherwise the current working directory.
//
// A missing path is reported by the caller, which surfaces it as exit code 3
// with the path in the message rather than as a bare stat error.
func resolveArg(args []string) (string, error) {
	raw := ""
	if len(args) > 0 {
		raw = args[0]
	}
	if raw == "" {
		cwd, err := getwd()
		if err != nil {
			return "", err
		}
		raw = cwd
	}
	return filepath.Abs(raw)
}
