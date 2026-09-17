package cli

import (
	"github.com/spf13/cobra"

	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

// newInstallCmd returns the "install" command, built from
// github.com/strongo/cli-helpers/cliinstall/cobracmd against cover100's own
// compiled catalog entry (cli-install#req:host-identity-from-catalog).
// `cover100 install` lists the fleet CLIs relevant to cover100 (codegrapher,
// wb) with their live status, and `cover100 install <name>...` installs
// them the same way cover100 itself was installed. cobracmd.New panics if
// "cover100" is absent from the compiled catalog — a programming error
// TestNewInstallCmd_Registration below catches, never a runtime state a
// user sees.
func newInstallCmd() *cobra.Command {
	return cobracmd.New(cobracmd.CommandOptions{
		Short:  "List and install fleet CLIs relevant to cover100",
		Errors: installErrors{},
		HostID: binaryName,
	})
}

// installErrors implements cobracmd.ErrorMapper for cover100's own install
// command (cli-install#req:host-owned-exit-codes). Unlike a host with a
// single generic exit code, cover100 already distinguishes a usage mistake
// (pkg/exitcode.InvalidArgs, 2) from an unexpected runtime failure
// (pkg/exitcode.Unexpected, 10) for every other command it has, so this
// mapper follows task-11's own two-bucket contract exactly:
// selfupdate.KindUnknownTarget — an EXPLICIT branch, never a self-update
// default — maps to cover100's usage exit code, and every other kind (the
// other two cli-install-only kinds, every self-update-shared kind, and an
// already-*cobracmd.UsageError for a bad --format or --all-with-names) maps
// to cover100's general failure exit code.
//
// A nil err IS a real, reachable call on the ordinary success and dry-run
// path, not just a defensive guard: cliinstall/cobracmd v0.20.0's
// runInstall calls mapFailure(opts, plan.Failure()) and mapFailure(opts,
// result.Failure()) unconditionally, and both return nil for a fully
// successful batch, so installErrors.Failure(nil) runs on every successful
// `cover100 install` and `cover100 install <name> --dry-run`. Feedback for
// cli-helpers (known bug, not yet fixed at v0.20.0): mapFailure itself
// should short-circuit nil before calling opts.Errors.Failure, matching
// what ErrorMapper.Failure's own doc comment already promises ("maps a
// non-nil command error").
type installErrors struct{}

// Failure maps err into cover100's own exit-code convention.
func (installErrors) Failure(err error) error {
	if err == nil {
		return nil
	}
	if selfupdate.KindOf(err) == selfupdate.KindUnknownTarget {
		return exitcode.Wrap(exitcode.InvalidArgs, err.Error(), err)
	}
	return exitcode.Wrap(exitcode.Unexpected, err.Error(), err)
}
