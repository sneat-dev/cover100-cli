package cli

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/selfupdate"
	selfupdatecmd "github.com/strongo/cli-helpers/selfupdate/cobracmd"
)

// selfUpdateConfig describes this CLI to the shared self-update machinery,
// built from cover100's own compiled-in catalog entry
// (cliinstall.ByID("cover100")) rather than a hand-written duplicate
// (cli-install#req:catalog-identity-single-source: "A host's self-update
// SHOULD build its Config from its own entry so its self-update and every
// other host's install <that cli> resolve releases identically"). The
// catalog entry (cliinstall/catalog_cover100.go in strongo/cli-helpers)
// carries the repository, supported platforms and version-probe args this
// function used to hand-roll, and no Managers: cover100 is published only
// as plain GitHub release archives (see .goreleaser.yml), so an install
// path that matches nothing is reported as Ambiguous and refused, which is
// the safe outcome — self-update never overwrites a binary it cannot
// classify. HTTPClient is the one thing only self-update itself needs,
// which the same REQ allows a host to add on top of its entry.
func selfUpdateConfig() selfupdate.Config {
	entry, ok := catalogEntryByID(binaryName)
	if !ok {
		// A host id absent from the compiled catalog is a programming error
		// caught by this package's own tests, never a runtime state a user
		// can trigger (cli-install#req:host-identity-from-catalog) --
		// matching specscore's and chatwright's own panic, rather than
		// silently running with an empty selfupdate.Config.
		panic(fmt.Sprintf("cliinstall: no catalog entry for %q", binaryName))
	}
	cfg := entry.Config(buildInfo.Version)
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	return cfg
}

// selfUpdateConfigFunc is a seam over selfUpdateConfig so tests can point a
// full command execution at an httptest.Server instead of the real GitHub
// API. newUpgradeCmd (upgrade.go) resolves its own HostConfig through this
// SAME seam, so `cover100 self-update` and `cover100 upgrade cover100`
// always build from the identical Config
// (cli-install#req:self-update-equals-upgrade-self,
// cli-install#req:host-target-is-running-binary), by construction rather
// than by two copies staying in sync.
var selfUpdateConfigFunc = selfUpdateConfig

// catalogEntryByID is a test seam over cliinstall.ByID so the defensive
// panic in selfUpdateConfig (a host id absent from the compiled catalog,
// which never happens in production -- cover100's own catalog entry always
// exists) is exercisable, matching specscore's and chatwright's own
// identical seam.
var catalogEntryByID = cliinstall.ByID

// newSelfUpdateCmd builds the self-update verb from the shared library so
// cover100 does not hand-roll release lookup, checksum verification or the
// atomic swap.
//
// Errors is installErrors{} (install.go) -- the SAME mapper install and
// upgrade use, not a bespoke passthrough. Before this fix self-update let
// every error pass through unchanged, which Fatal (root.go) defaults to
// exit 1 for any error without its own ExitCode() method -- a code that
// does not even appear in cover100's own exit-code table
// (spec/features/cli-command-surface#req:exit-codes-and-output: `2`/`10`,
// never `1`) and that silently disagreed with `upgrade cover100`'s exit `10`
// for the identical failure (cli-install#req:self-update-equals-upgrade-self:
// "for every outcome ... exit codes stay those self-update already
// documents"). One shared mapper makes that equality hold by construction.
func newSelfUpdateCmd() *cobra.Command {
	return selfupdatecmd.New(selfUpdateConfigFunc(), selfupdatecmd.CommandOptions{
		Use:        "self-update",
		Short:      "Update the installed cover100 binary to the latest release",
		Aliases:    []string{"update"},
		JSONFormat: true,
		Errors:     installErrors{},
	})
}
