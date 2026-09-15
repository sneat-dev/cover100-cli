// Package executil prepares child processes so that cancelling their context
// actually stops them.
//
// cover100 shells out to `go test`, `npx`, jest and vitest. All of those spawn
// their own children — compilers, workers, test binaries — which inherit the
// parent's stdout and stderr. Killing only the direct child therefore leaves
// two problems behind: the orphaned tree keeps consuming CPU and keeps running
// the user's test suite after the timeout, and cmd.Wait blocks on the inherited
// pipes until those orphans finally exit. A 60s timeout was measured taking 16
// minutes to return on a large Go module for exactly that reason.
package executil

import (
	"os/exec"
	"time"
)

// WaitDelay bounds how long Wait keeps waiting on the command's I/O pipes after
// the process has been killed or its context is done. It is the backstop that
// makes a timeout mean a timeout: even if a grandchild somehow survives the
// process-group kill, cover100 stops waiting for it.
const WaitDelay = 5 * time.Second

// Prepare makes cmd cancellable as a whole tree.
//
// Call it on every command built with exec.CommandContext. It sets the pipe
// wait bound and, on platforms that have process groups, arranges for
// cancellation to signal every process the command started rather than only
// the one it started directly.
func Prepare(cmd *exec.Cmd) {
	cmd.WaitDelay = WaitDelay
	configureProcessGroup(cmd)
}
