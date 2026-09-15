//go:build windows

package executil

import "os/exec"

// configureProcessGroup is a no-op on Windows: exec.CommandContext's default
// Cancel already terminates the child process, and there is no portable
// equivalent of the Unix process-group signal used elsewhere in this package.
// The WaitDelay set by Prepare still bounds the pipe wait, so a cancelled
// command returns promptly even if a grandchild outlives its parent.
func configureProcessGroup(_ *exec.Cmd) {}
