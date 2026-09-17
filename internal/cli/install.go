package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"
	selfupdatecmd "github.com/strongo/cli-helpers/selfupdate/cobracmd"

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

// installErrors is cover100's ONE error mapper, shared by self-update
// (self_update.go), install (this file) and upgrade (upgrade.go)
// (cli-install#req:host-owned-exit-codes: "The upgrade command MUST use the
// same error mapper"; cli-install#req:self-update-equals-upgrade-self: "for
// every outcome ... exit codes stay those self-update already documents").
// Before this fix self-update used a bespoke passthrough that let `Fatal`
// (root.go) default an unmapped error to exit 1 -- a code absent from
// cover100's own two-bucket contract entirely -- so the SAME failure kind
// exited differently from self-update than from `upgrade cover100`. One
// mapper for all three commands makes that equality hold by construction.
//
// Unlike a host with a single generic exit code, cover100 already
// distinguishes a usage mistake (pkg/exitcode.InvalidArgs, 2) from an
// unexpected runtime failure (pkg/exitcode.Unexpected, 10) for every other
// command it has (spec/features/cli-command-surface#req:exit-codes-and-output).
// selfupdate.KindUnknownTarget and an already-*cobracmd.UsageError (bad
// `--format`, `--all` combined with names) or *selfupdatecmd.UsageError
// (self-update's own invalid-flag shape) — EXPLICIT branches, never a
// self-update default — map to cover100's usage exit code; every other
// kind (the other two cli-install-only kinds and every self-update-shared
// kind) maps to cover100's general failure exit code.
//
// cliinstall/cobracmd v0.21.0's mapFailure short-circuits a nil err before
// ever calling opts.Errors.Failure (the fix for the known v0.20.0 bug this
// comment used to document), so Failure is never called with nil through
// that path anymore; the guard below stays only because it is trivially
// free and keeps this method nil-safe for any direct caller, including
// TestInstallErrorsFailure_NilReturnsNil.
type installErrors struct{}

// Failure maps err into cover100's own exit-code convention.
func (installErrors) Failure(err error) error {
	if err == nil {
		return nil
	}
	var installUsage *cobracmd.UsageError
	if errors.As(err, &installUsage) {
		return exitcode.Wrap(exitcode.InvalidArgs, err.Error(), err)
	}
	var selfUpdateUsage *selfupdatecmd.UsageError
	if errors.As(err, &selfUpdateUsage) {
		return exitcode.Wrap(exitcode.InvalidArgs, err.Error(), err)
	}
	if selfupdate.KindOf(err) == selfupdate.KindUnknownTarget {
		return exitcode.Wrap(exitcode.InvalidArgs, err.Error(), err)
	}
	return exitcode.Wrap(exitcode.Unexpected, err.Error(), err)
}

// UpdateAvailable implements selfupdate/cobracmd.ErrorMapper (self-update's
// own interface, distinct from cliinstall/cobracmd.ErrorMapper, requires
// this method too): an available update is informational, not a failure,
// exactly as it was under self-update's previous bespoke mapper.
func (installErrors) UpdateAvailable(selfupdate.CheckResult) error { return nil }
