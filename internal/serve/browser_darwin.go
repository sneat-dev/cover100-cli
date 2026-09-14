//go:build darwin

package serve

import "os/exec"

// browserCommand returns the platform's "open this URL" launcher. See the
// Windows file for why this is build-tagged rather than a runtime.GOOS switch.
func browserCommand(url string) *exec.Cmd {
	return exec.Command("open", url)
}
