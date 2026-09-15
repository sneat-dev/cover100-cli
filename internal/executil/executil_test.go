package executil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPrepare_SetsThePipeWaitBoundWithoutClobberingTheCommand(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	cmd.Dir = "/tmp"

	Prepare(cmd)

	if cmd.WaitDelay != WaitDelay {
		t.Errorf("WaitDelay = %v, want %v: a cancelled command must not wait forever on inherited pipes", cmd.WaitDelay, WaitDelay)
	}
	if cmd.Dir != "/tmp" {
		t.Errorf("Dir = %q, want the caller's value preserved", cmd.Dir)
	}
	if cmd.Path == "" || len(cmd.Args) != 2 {
		t.Errorf("Prepare() altered the command: Path=%q Args=%v", cmd.Path, cmd.Args)
	}
}

// TestPrepare_CancellingStopsTheWholeProcessTree is the regression test for the
// bug this package exists to fix: a 60s timeout on a large Go module took 16
// minutes to return because `go test`'s grandchildren inherited the output pipe
// and kept running after only the direct child was killed.
//
// The stand-in tree is a shell that backgrounds a subshell. The subshell
// inherits stdout — so it holds the pipe — and would touch a marker file if it
// survived cancellation.
func TestPrepare_CancellingStopsTheWholeProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in process tree is a POSIX shell script")
	}

	marker := filepath.Join(t.TempDir(), "grandchild-survived")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf("( sleep 3; touch %q ) & sleep 30", marker))
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	Prepare(cmd)

	started := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Let the tree reach the point where both processes exist, then cancel as a
	// timeout would.
	time.Sleep(300 * time.Millisecond)
	cancel()

	if err := cmd.Wait(); err == nil {
		t.Error("Wait() = nil, want an error: the command was killed")
	}
	elapsed := time.Since(started)

	// Without the pipe bound this waits for the surviving grandchild's full 30
	// seconds. The allowance covers WaitDelay plus scheduling slack.
	if limit := WaitDelay + 3*time.Second; elapsed > limit {
		t.Errorf("Wait() returned after %v, want under %v: the inherited pipe was not released", elapsed, limit)
	}

	// The stronger claim: the grandchild was actually stopped, not just
	// abandoned. It would create the marker at the 3 second mark.
	time.Sleep(4 * time.Second)
	if _, err := os.Stat(marker); err == nil {
		t.Error("the grandchild outlived cancellation and completed its work; the process group was not signalled")
	}
}
