package cli

import (
	"os"
	"path/filepath"
)

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
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		raw = cwd
	}
	return filepath.Abs(raw)
}
