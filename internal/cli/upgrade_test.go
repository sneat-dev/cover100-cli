package cli

import (
	"bytes"
	"testing"
)

func TestNewUpgradeCmd_RegistrationAndFlags(t *testing.T) {
	cmd := newUpgradeCmd()

	if cmd.Name() != "upgrade" {
		t.Errorf("Name = %q, want %q", cmd.Name(), "upgrade")
	}

	for _, flag := range []string{"all", "check", "yes", "dry-run", "format"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not found on upgrade command", flag)
		}
	}

	if cmd.Flags().Lookup("dir") != nil {
		t.Error("upgrade command should not have --dir flag")
	}
}

func TestNewUpgradeCmd_InvalidFormat(t *testing.T) {
	cmd := newUpgradeCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with invalid --format expected error, got nil")
	}
}

func TestNewUpgradeCmd_AllWithArgs(t *testing.T) {
	cmd := newUpgradeCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--all", "specscore"})

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() with --all and positional args expected error, got nil")
	}
}
