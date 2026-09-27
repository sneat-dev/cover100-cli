package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/skillsync"
	skillscmd "github.com/strongo/cli-helpers/skillsync/cobracmd"

	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

func TestSkillsCmd_Registration(t *testing.T) {
	cmd := newSkillsCmd()
	if cmd.Name() != "skills" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "skills")
	}
	sub := cmd.Commands()
	hasSync := false
	for _, c := range sub {
		if c.Name() == "sync" {
			hasSync = true
			break
		}
	}
	if !hasSync {
		t.Errorf("skills command does not contain sync subcommand; subcommands = %v", sub)
	}
}

func TestSkillsCmd_RegisteredAtRoot(t *testing.T) {
	rootCmd, fangOpts := NewRootCmd(nil)
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"skills", "--help"})

	err := executeWithPanicRecovery(rootCmd, fangOpts...)
	if err != nil {
		t.Fatalf("cover100 skills --help failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "skills") {
		t.Errorf("help output does not mention skills:\n%s", stdout.String())
	}
}

func TestSkillsConfig_LoadsEmbeddedSkills(t *testing.T) {
	cfg, err := newSkillsConfig()
	if err != nil {
		t.Fatalf("newSkillsConfig failed: %v", err)
	}
	if cfg.CLI.Name != "cover100" || cfg.CLI.Publisher != "cover100" {
		t.Errorf("CLI identity = %+v, want cover100/cover100", cfg.CLI)
	}
	if len(cfg.Bundles) == 0 {
		t.Fatal("cfg.Bundles is empty")
	}
	bundle := cfg.Bundles[0]
	if bundle.Plugin.Name != "cover100" || bundle.Plugin.Publisher != "cover100" {
		t.Errorf("Bundle plugin identity = %+v, want cover100/cover100", bundle.Plugin)
	}
	if bundle.Source.Repository != "github.com/sneat-dev/cover100-cli" {
		t.Errorf("Bundle repository = %q, want github.com/sneat-dev/cover100-cli", bundle.Source.Repository)
	}
	if bundle.Source.Path != "ai/skills" {
		t.Errorf("Bundle path = %q, want ai/skills", bundle.Source.Path)
	}
	if bundle.Source.Digest == "" {
		t.Error("Bundle digest is empty")
	}
}

func TestSkillsSyncErrors_Failure(t *testing.T) {
	errUsage := (skillsSyncErrors{}).Failure(&skillscmd.UsageError{Err: errors.New("bad arg")})
	var codedUsage *exitcode.Error
	if !errors.As(errUsage, &codedUsage) {
		t.Fatalf("Failure(usage) did not return *exitcode.Error: %v", errUsage)
	}
	if codedUsage.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("code = %d, want InvalidArgs (%d)", codedUsage.ExitCode(), exitcode.InvalidArgs)
	}

	errOther := (skillsSyncErrors{}).Failure(errors.New("sync failed"))
	var codedOther *exitcode.Error
	if !errors.As(errOther, &codedOther) {
		t.Fatalf("Failure(other) did not return *exitcode.Error: %v", errOther)
	}
	if codedOther.ExitCode() != exitcode.Unexpected {
		t.Errorf("code = %d, want Unexpected (%d)", codedOther.ExitCode(), exitcode.Unexpected)
	}
}

func TestSkillsSyncErrors_Conflict(t *testing.T) {
	report := skillsync.Report{
		Dir: "/tmp/fake-harness/skills",
		Changes: []skillsync.Change{
			{Name: "cover100", Action: skillsync.Conflict, Reason: "owned by someone else"},
		},
	}
	err := (skillsSyncErrors{}).Conflict(report)
	var coded *exitcode.Error
	if !errors.As(err, &coded) {
		t.Fatalf("Conflict did not return *exitcode.Error: %v", err)
	}
	if coded.ExitCode() != exitcode.Unexpected {
		t.Errorf("code = %d, want Unexpected (%d)", coded.ExitCode(), exitcode.Unexpected)
	}
	if !strings.Contains(coded.Error(), "1 skill(s) could not be installed") {
		t.Errorf("unexpected error message: %q", coded.Error())
	}
}

func TestSkillsConfig_WithValidBuildInfo(t *testing.T) {
	prevBuildInfo := buildInfo
	buildInfo.Commit = "0123456789abcdef0123456789abcdef01234567"
	buildInfo.Version = "v1.2.3"
	t.Cleanup(func() { buildInfo = prevBuildInfo })

	cfg, err := newSkillsConfig()
	if err != nil {
		t.Fatalf("newSkillsConfig failed: %v", err)
	}
	if len(cfg.Bundles) == 0 {
		t.Fatal("no bundles found")
	}
	if cfg.Bundles[0].Source.Revision != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("Revision = %q", cfg.Bundles[0].Source.Revision)
	}
	if cfg.Bundles[0].Source.Version != "v1.2.3" {
		t.Errorf("Version = %q", cfg.Bundles[0].Source.Version)
	}
}

