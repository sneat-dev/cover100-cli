package cli

import (
	"github.com/spf13/cobra"

	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
)

// newUpgradeCmd returns the "upgrade" command, built from
// github.com/strongo/cli-helpers/cliinstall/cobracmd against cover100's own
// compiled catalog entry (cli-install#req:host-identity-from-catalog).
// `cover100 upgrade` (no arguments) reports every installed catalog CLI plus
// cover100 itself — current version, latest stable release, and verdict —
// without changing anything; `cover100 upgrade --all`/`cover100 upgrade
// <name>...` upgrade what the report showed. cover100 is always upgraded
// last, classified and versioned from its OWN self-update Config (never a
// PATH probe of its own binary): HostConfig resolves through the SAME
// selfUpdateConfigFunc seam newSelfUpdateCmd uses, so `cover100
// self-update` and `cover100 upgrade cover100` reach the exact same library
// call (cli-install#req:self-update-equals-upgrade-self,
// cli-install#req:host-target-is-running-binary). cover100's self-update
// configures no after-update hook, so HostAfterUpdate is left nil.
// cobracmd.NewUpgrade panics if "cover100" is absent from the compiled
// catalog — a programming error TestNewUpgradeCmd_Registration below
// catches, never a runtime state a user sees.
func newUpgradeCmd() *cobra.Command {
	return cobracmd.NewUpgrade(cobracmd.UpgradeCommandOptions{
		Short:      "Upgrade installed fleet CLIs, including cover100",
		Errors:     installErrors{},
		HostID:     binaryName,
		HostConfig: selfUpdateConfigFunc(),
		Env:        upgradeEnv,
	})
}

// upgradeEnv is a seam over cobracmd.UpgradeCommandOptions.Env: its zero
// value (PathDirs == nil) is what cobracmd's own resolveEnv treats as "use
// the real cliinstall.DefaultInstallEnv()" in production. Tests override
// this to a fully offline Env so the bare report never probes -- and never
// makes a real release lookup for -- any OTHER fleet CLI genuinely
// installed on this machine's real PATH (cli-install#req:no-network-in-
// tests).
var upgradeEnv cliinstall.InstallEnv
