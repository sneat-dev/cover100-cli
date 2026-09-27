package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/skillsync"
	skillscmd "github.com/strongo/cli-helpers/skillsync/cobracmd"
	"github.com/strongo/cli-helpers/skillsync/githubrelease"

	"github.com/sneat-dev/cover100-cli/ai"
	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

const (
	cover100SkillsPluginVersion = "0.0.0"
	cover100SkillsUnknownSource = "0000000000000000000000000000000000000000"
)

var (
	cover100SkillsCLI    = skillsync.Identity{Publisher: "cover100", Name: "cover100"}
	cover100SkillsPlugin = skillsync.PluginIdentity{Publisher: "cover100", Name: "cover100"}

	skillsFS             = ai.SkillsFS
	skillsSubFS          = fs.Sub
	skillsDigest         = skillsync.Digest
	skillsEmbeddedBundle = skillsync.EmbeddedBundle
)

func newSkillsConfig() (skillsync.Config, error) {
	source, err := skillsSubFS(skillsFS, "skills")
	if err != nil {
		return skillsync.Config{}, err
	}
	digest, err := skillsDigest(source)
	if err != nil {
		return skillsync.Config{}, err
	}
	revision := buildInfo.Commit
	if len(revision) != 40 {
		revision = cover100SkillsUnknownSource
	}
	pluginVersion := buildInfo.Version
	if _, err := skillsync.CompareVersions(pluginVersion, pluginVersion); err != nil {
		pluginVersion = cover100SkillsPluginVersion
	}
	bundle, err := skillsEmbeddedBundle(skillsync.BundleDescriptor{
		Plugin: cover100SkillsPlugin,
		Source: skillsync.Source{
			Repository: "github.com/sneat-dev/cover100-cli",
			Path:       "ai/skills",
			Revision:   revision,
			Version:    pluginVersion,
			Digest:     digest,
		},
	}, source)
	if err != nil {
		return skillsync.Config{}, err
	}
	return skillsync.Config{
		CLI:            cover100SkillsCLI,
		CurrentVersion: buildInfo.Version,
		Bundles:        []skillsync.Bundle{bundle},
	}, nil
}

func newSkillsCmd() *cobra.Command {
	cfg, cfgErr := newSkillsConfig()
	options := skillscmd.CommandOptions{
		Use:    "skills",
		Short:  "Install cover100's Agent Skills into a harness's skills directory",
		Errors: skillsSyncErrors{},
		Resolver: skillsync.ReleaseResolver{
			Source:         githubrelease.Source{},
			CurrentVersion: cfg.CurrentVersion,
		},
	}
	cmd := skillscmd.New(cfg, options)
	cmd.Long = `Install cover100's Agent Skills into a harness's skills directory.

cover100 ships agent-facing skills under ai/skills/, and harnesses (Claude Code, Cursor, Codex)
auto-discover them once synchronized.

'cover100 skills sync' copies every shipped skill into each present harness's skills directory.`
	if cfgErr != nil {
		cmd.RunE = func(*cobra.Command, []string) error {
			return skillsSyncErrors{}.Failure(fmt.Errorf("prepare embedded cover100 skills: %w", cfgErr))
		}
	}
	return cmd
}

type skillsSyncErrors struct{}

func (skillsSyncErrors) Failure(err error) error {
	var usage *skillscmd.UsageError
	if errors.As(err, &usage) {
		return exitcode.InvalidArgsErrorf("%v", err)
	}
	return exitcode.UnexpectedErrorf("skills: %v", err)
}

func (skillsSyncErrors) Conflict(report skillsync.Report) error {
	return exitcode.UnexpectedErrorf(
		"skills: %d skill(s) could not be installed because another plugin or an unmanaged directory already owns the name; see %s",
		len(report.Names(skillsync.Conflict)), report.Dir)
}
