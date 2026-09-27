package cli

import (
	"bytes"
	"testing"
)

func TestNewUninstallCmd_RegistrationAndFlags(t *testing.T) {
	cmd := newUninstallCmd()

	if cmd.Name() != "uninstall" {
		t.Errorf("Name = %q, want %q", cmd.Name(), "uninstall")
	}

	for _, flag := range []string{"all", "yes", "dry-run", "purge", "format"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not found on uninstall command", flag)
		}
	}

	if cmd.Flags().Lookup("dir") != nil {
		t.Error("uninstall command should not have --dir flag")
	}
}

func TestNewUninstallCmd_InvalidFormat(t *testing.T) {
	cmd := newUninstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--all", "--format", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with invalid --format expected error, got nil")
	}
}

func TestNewUninstallCmd_AllWithArgs(t *testing.T) {
	cmd := newUninstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--all", "specscore"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with --all and positional args expected error, got nil")
	}
}

func TestNewUninstallCmd_NoArgsWithoutAll(t *testing.T) {
	cmd := newUninstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with no args and without --all expected error, got nil")
	}
}

func TestNewUninstallCmd_DryRunAll(t *testing.T) {
	cmd := newUninstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--all", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() dry-run all failed: %v", err)
	}
}

func TestNewUninstallCmd_DryRunJSON(t *testing.T) {
	cmd := newUninstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--all", "--dry-run", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() dry-run JSON failed: %v", err)
	}
	if out.Len() == 0 {
		t.Error("expected JSON output on stdout, got empty")
	}
}
