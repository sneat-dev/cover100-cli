package cli

import (
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/selfupdate"
	selfupdatecmd "github.com/strongo/cli-helpers/selfupdate/cobracmd"
)

// repository is the GitHub repository cover100 releases from. The self-update
// verb downloads that repository's release archives, which is why the
// GoReleaser configuration must publish plain archives plus checksums.txt.
const repository = "sneat-dev/cover100-cli"

// selfUpdateConfig describes this CLI to the shared self-update machinery.
func selfUpdateConfig() selfupdate.Config {
	return selfupdate.Config{
		BinaryName:     binaryName,
		Repository:     repository,
		CurrentVersion: buildInfo.Version,
		// UndeterminedVersions is deliberately left unset: Config defaults it to
		// []string{"dev"}, which is exactly the placeholder buildinfo.Get
		// returns for an unstamped local build.
		// No package-manager installs are registered: this CLI is published
		// only as plain GitHub release archives (see .goreleaser.yml), so a
		// manager entry would be dead configuration. An install path that
		// matches nothing is reported as Ambiguous and refused, which is the
		// safe outcome — self-update never overwrites a binary it cannot
		// classify.
		// Every combination GoReleaser publishes (see .goreleaser.yml): a
		// platform missing here would be refused an update it could have had.
		SupportedPlatforms: []selfupdate.Platform{
			{GOOS: "darwin", GOARCH: "amd64"},
			{GOOS: "darwin", GOARCH: "arm64"},
			{GOOS: "linux", GOARCH: "amd64"},
			{GOOS: "linux", GOARCH: "arm64"},
			{GOOS: "windows", GOARCH: "amd64"},
			{GOOS: "windows", GOARCH: "arm64"},
		},
		// The root command's own --version flag is what the post-swap probe
		// reads, so the probe arguments match cobra's built-in flag.
		VersionProbeArgs: []string{"--version"},
		HTTPClient:       &http.Client{Timeout: 30 * time.Second},
	}
}

// newSelfUpdateCmd builds the self-update verb from the shared library so
// cover100 does not hand-roll release lookup, checksum verification or the
// atomic swap.
func newSelfUpdateCmd() *cobra.Command {
	return selfupdatecmd.New(selfUpdateConfig(), selfupdatecmd.CommandOptions{
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
