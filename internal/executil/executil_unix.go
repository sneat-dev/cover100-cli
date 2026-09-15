//go:build !windows

package executil

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the child in its own process group and makes
// cancellation signal that whole group.
//
// Without this, cancelling the context kills only `go test` or `npx`, and the
// compilers and test binaries it spawned keep running. Signalling the negative
// pid reaches every process in the group, which is what stops the work rather
// than merely detaching from it.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// A negative pid addresses the process group created by Setpgid. The
		// process is always started by the time Cancel runs — os/exec installs
		// this hook after Start assigns cmd.Process — so cmd.Process is never
		// nil here and needs no guard.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
