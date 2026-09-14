// Package cli builds cover100's command tree.
//
// Each verb lives in its own file. This file wires the root command, the
// fleet-standard fang front-end, and the process exit-code convention.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
	"github.com/strongo/buildinfo"
	"github.com/strongo/buildinfo/fangcmd"

	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

// buildInfo is this process's resolved build identity, resolved once at
// package init from link-time stamps or, for a plain `go build`, from Go's own
// runtime/debug fallback. The `--version` flag and the `version` subcommand
// are both wired from this one value by fangcmd.Wire, so they cannot disagree.
var buildInfo = buildinfo.Get(binaryName)

// binaryName is the command name, and also the name GoReleaser publishes.
const binaryName = "cover100"

// osExit is a test seam over os.Exit so exit codes can be asserted in-process.
var osExit = os.Exit

// Run executes cover100 with the given arguments (including argv[0]) and
// returns the error the caller should turn into an exit code.
func Run(args []string, assets fs.FS) error {
	root, fangOpts := NewRootCmd(assets)
	if len(args) > 1 {
		root.SetArgs(args[1:])
	}
	return executeWithPanicRecovery(root, fangOpts...)
}

// NewRootCmd builds the root command and the fang options it must be executed
// with. Callers MUST pass the returned options into fang.Execute alongside the
// command, so the version flag and version subcommand stay in agreement.
func NewRootCmd(assets fs.FS) (*cobra.Command, []fang.Option) {
	opts := &collectOptions{}
	root := &cobra.Command{
		Use:   binaryName + " [path]",
		Short: "Visualise Go and TypeScript test coverage as an interactive treemap",
		Long: `cover100 collects test coverage from Go modules and Node packages, normalizes
it into a single JSON document, and opens a zoomable treemap of it in your
browser. Nothing is uploaded and no server is left running behind you.

Boxes are sized by the metric you choose and coloured by coverage, from red at
0% through amber at 50% to green at 100%. Click a box to drill in, Esc to come
back out.

Go's cover profiles carry no function-level data, so function coverage for Go
files is reported as "not measured" rather than estimated.`,
		Version:       buildInfo.Short(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runCollect(ctx, cmd, args, assets, opts)
		},
	}
	root.SetErr(os.Stderr)
	opts.bind(root)

	root.AddCommand(newSelfUpdateCmd())

	fangOpts := fangcmd.Wire(root, buildInfo)
	return root, fangOpts
}

// executeWithPanicRecovery runs the command tree, converting a panic into an
// exit-code-10 error with the stack on stderr instead of a bare runtime crash.
func executeWithPanicRecovery(root *cobra.Command, extraOpts ...fang.Option) (returnErr error) {
	var (
		panicVal   any
		panicStack []byte
	)
	opts := append([]fang.Option{fang.WithErrorHandler(silentErrorHandler)}, extraOpts...)
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicVal = r
				panicStack = debug.Stack()
			}
		}()
		returnErr = fang.Execute(context.Background(), root, opts...)
	}()

	if panicVal != nil {
		_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n\n%s\n", panicVal, panicStack)
		return exitcode.UnexpectedErrorf("panic recovered: %v", panicVal)
	}
	return returnErr
}

// silentErrorHandler stops fang from rendering the error itself. Fatal is the
// single place that reports an error and picks the exit code, so nothing is
// printed twice.
func silentErrorHandler(io.Writer, fang.Styles, error) {}

// Fatal prints err and exits with the code it carries, defaulting to 1 for an
// error that carries none.
func Fatal(err error) {
	if err == nil {
		return
	}
	_, _ = os.Stderr.WriteString(err.Error() + "\n")

	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		osExit(coder.ExitCode())
		return
	}
	osExit(1)
}
