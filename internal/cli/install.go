package cli

import (
	"github.com/spf13/cobra"
	installcmd "github.com/strongo/cli-helpers/cliinstall/cobracmd"
)

// newInstallCmd builds the install verb from the shared library targeting the
// cover100 host id in the compiled fleet catalog.
func newInstallCmd() *cobra.Command {
	return installcmd.New(installcmd.CommandOptions{
		HostID: binaryName,
		Errors: passthroughErrors{},
	})
}
