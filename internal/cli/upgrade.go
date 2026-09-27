package cli

import (
	"github.com/spf13/cobra"
	installcmd "github.com/strongo/cli-helpers/cliinstall/cobracmd"
)

// newUpgradeCmd builds the upgrade verb from the shared library targeting the
// cover100 host id in the compiled fleet catalog.
func newUpgradeCmd() *cobra.Command {
	return installcmd.NewUpgrade(installcmd.UpgradeCommandOptions{
		HostID:     binaryName,
		HostConfig: selfUpdateConfig(),
		Errors:     passthroughErrors{},
	})
}
