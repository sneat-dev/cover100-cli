//go:build windows

package serve

import "os/exec"

// browserCommand returns the platform's "open this URL" launcher.
//
// Each platform gets its own file rather than one `switch runtime.GOOS`:
// runtime.GOOS is a compile-time constant, so on any single platform every
// other arm of such a switch is dead code that the coverage tool still counts.
// Splitting it per platform keeps each build's coverage honest, and CI's
// Windows job is what proves this file.
func browserCommand(url string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
}
