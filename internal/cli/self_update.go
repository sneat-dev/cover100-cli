package cli

import (
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
	entry, _ := cliinstall.ByID(binaryName)
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

// newSelfUpdateCmd builds the self-update verb from the shared library so
// cover100 does not hand-roll release lookup, checksum verification or the
// atomic swap.
func newSelfUpdateCmd() *cobra.Command {
	return selfupdatecmd.New(selfUpdateConfigFunc(), selfupdatecmd.CommandOptions{
		Use:        "self-update",
		Short:      "Update the installed cover100 binary to the latest release",
		Aliases:    []string{"update"},
		JSONFormat: true,
		// No exit-code contract of its own: every error passes through
		// unchanged and an available update is informational, not a failure.
		Errors: passthroughErrors{},
	})
}

// passthroughErrors is this CLI's self-update ErrorMapper.
type passthroughErrors struct{}

func (passthroughErrors) Failure(err error) error { return err }

func (passthroughErrors) UpdateAvailable(selfupdate.CheckResult) error { return nil }
