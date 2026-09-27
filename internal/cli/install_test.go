package cli

import (
	"bytes"
	"testing"
)

func TestNewInstallCmd_RegistrationAndFlags(t *testing.T) {
	cmd := newInstallCmd()

	if cmd.Name() != "install" {
		t.Errorf("Name = %q, want %q", cmd.Name(), "install")
	}

	for _, flag := range []string{"all", "yes", "dry-run", "dir", "format"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not found on install command", flag)
		}
	}
}

func TestNewInstallCmd_InvalidFormat(t *testing.T) {
	cmd := newInstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with invalid --format expected error, got nil")
	}
}

func TestNewInstallCmd_AllWithArgs(t *testing.T) {
	cmd := newInstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--all", "specscore"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with --all and positional args expected error, got nil")
	}
}

func TestNewInstallCmd_ListOffline(t *testing.T) {
	cmd := newInstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() list failed: %v", err)
	}
}

func TestNewInstallCmd_ListJSON(t *testing.T) {
	cmd := newInstallCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() list JSON failed: %v", err)
	}
	if out.Len() == 0 {
		t.Error("expected JSON output on stdout, got empty")
	}
}
