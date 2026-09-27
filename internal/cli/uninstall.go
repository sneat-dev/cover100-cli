package cli

import (
	"github.com/spf13/cobra"
	installcmd "github.com/strongo/cli-helpers/cliinstall/cobracmd"
)

// newUninstallCmd builds the uninstall verb from the shared library targeting the
// cover100 host id in the compiled fleet catalog.
func newUninstallCmd() *cobra.Command {
	return installcmd.NewUninstall(installcmd.UninstallCommandOptions{
		HostID: binaryName,
		Errors: fleetErrors{},
	})
}
