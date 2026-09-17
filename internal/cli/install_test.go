package cli

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

// --- install.go: newInstallCmd registration ---

func TestNewInstallCmd_Registration(t *testing.T) {
	cmd := newInstallCmd()
	if !strings.HasPrefix(cmd.Use, "install") {
		t.Errorf("Use = %q, want it to start with install", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short is empty, want a description")
	}
	for _, name := range []string{"all", "yes", "dry-run", "dir", "format"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q is not registered", name)
		}
	}
	if f := cmd.Flags().Lookup("yes"); f.Shorthand != "y" {
		t.Errorf("--yes shorthand = %q, want y", f.Shorthand)
	}
	// cli-install#req:update-alias-policy — "upgrade MUST NOT get an update
	// alias", and by the same policy install never gains one either: only
	// self-update keeps the update alias it already ships.
	for _, alias := range cmd.Aliases {
		if alias == "update" {
			t.Errorf("install command carries an %q alias; only self-update may keep it", alias)
		}
	}
}

// TestNewInstallCmd_ResolvesAgainstTheCatalog proves cobracmd.New does not
// panic for cover100's own HostID (cli-install#req:host-identity-from-
// catalog): "A host id absent from the catalog MUST be a programming error
// caught by the host's tests, not a runtime state users see." newInstallCmd
// already ran without panicking in every other test in this file; this
// test names the guarantee explicitly and confirms the catalog entry it
// resolves against is really cover100's own.
func TestNewInstallCmd_ResolvesAgainstTheCatalog(t *testing.T) {
	entry, ok := cliinstall.ByID(binaryName)
	if !ok {
		t.Fatalf("no catalog entry for %q", binaryName)
	}
	if entry.ID != binaryName {
		t.Errorf("catalog entry ID = %q, want %q", entry.ID, binaryName)
	}
}

// --- install.go: installErrors ---

// TestInstallErrorsFailure_NilReturnsNil proves the defensive nil guard:
// cliinstall/cobracmd v0.20.0's runInstall calls mapFailure(opts,
// plan.Failure()) and mapFailure(opts, result.Failure()) unconditionally,
// and both return nil for a fully successful batch (including a successful
// --dry-run), so installErrors.Failure(nil) is a real, reachable call on
// the ordinary success path, not just a defensive guard against a
// hypothetical caller (known cli-helpers v0.20.0 bug — see install.go's
// doc comment).
func TestInstallErrorsFailure_NilReturnsNil(t *testing.T) {
	if got := (installErrors{}).Failure(nil); got != nil {
		t.Errorf("Failure(nil) = %v, want nil", got)
	}
}

// TestInstallErrorsFailure_UnknownTargetMapsToUsageExitCode proves
// selfupdate.KindUnknownTarget is mapped through an EXPLICIT branch to
// cover100's own usage exit code (pkg/exitcode.InvalidArgs), never left to
// a self-update default branch (cli-install#req:host-owned-exit-codes),
// for both the shape cliinstall.Plan actually returns for an unknown name
// (a bare *selfupdate.Failure) and a *cliinstall.BatchFailure wrapping one.
func TestInstallErrorsFailure_UnknownTargetMapsToUsageExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "bare failure (cliinstall.Plan's own shape for an unknown name)",
			err: &selfupdate.Failure{
				Kind: selfupdate.KindUnknownTarget,
				Err:  errors.New("nosuchcli: not a known install target; valid ids: codegrapher, cover100, wb"),
			},
		},
		{
			name: "batch failure wrapping one unknown-target failure",
			err: &cliinstall.BatchFailure{Failures: []*selfupdate.Failure{
				{Kind: selfupdate.KindUnknownTarget, Err: errors.New("nosuchcli: not a known install target")},
			}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := (installErrors{}).Failure(c.err)
			if got == nil {
				t.Fatal("Failure(...) = nil, want a non-nil error")
			}
			var coder interface{ ExitCode() int }
			if !errors.As(got, &coder) {
				t.Fatalf("Failure(%v) = %v (%T), want an exitcode.Error", c.err, got, got)
			}
			if coder.ExitCode() != exitcode.InvalidArgs {
				t.Errorf("Failure(...) ExitCode() = %d, want %d (usage)", coder.ExitCode(), exitcode.InvalidArgs)
			}
			if !strings.Contains(got.Error(), "nosuchcli") {
				t.Errorf("Failure(...) = %q, want it to name the unknown target", got.Error())
			}
			if !errors.Is(got, c.err) {
				t.Errorf("Failure(...) does not wrap the original error for errors.Is")
			}
		})
	}
}

// TestInstallErrorsFailure_OtherKindsMapToFailureExitCode proves every
// failure that is NOT KindUnknownTarget — the other cli-install-only
// kinds, a self-update-shared kind, an already-*cobracmd.UsageError (a bad
// --format or --all-with-names), and a plain error — maps to cover100's
// own general failure exit code (pkg/exitcode.Unexpected), matching
// task-11's two-bucket contract ("KindUnknownTarget as usage and others as
// failure").
func TestInstallErrorsFailure_OtherKindsMapToFailureExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"no install dir", &selfupdate.Failure{Kind: selfupdate.KindNoInstallDir, Err: errors.New("no per-user bin directory on PATH")}},
		{"destination exists", &selfupdate.Failure{Kind: selfupdate.KindDestinationExists, Err: errors.New("destination already exists")}},
		{"checksum (self-update-shared kind)", &selfupdate.Failure{Kind: selfupdate.KindChecksum, Err: errors.New("checksum mismatch")}},
		{"already a usage error", &cobracmd.UsageError{Err: errors.New(`invalid --format "yaml": expected text or json`)}},
		{"plain error", errors.New("network unavailable")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := (installErrors{}).Failure(c.err)
			if got == nil {
				t.Fatal("Failure(...) = nil, want a non-nil error")
			}
			var coder interface{ ExitCode() int }
			if !errors.As(got, &coder) {
				t.Fatalf("Failure(%v) = %v (%T), want an exitcode.Error", c.err, got, got)
			}
			if coder.ExitCode() != exitcode.Unexpected {
				t.Errorf("Failure(...) ExitCode() = %d, want %d (failure)", coder.ExitCode(), exitcode.Unexpected)
			}
			if !errors.Is(got, c.err) {
				t.Errorf("Failure(...) does not wrap the original error for errors.Is")
			}
		})
	}
}

// TestInstallCmdNoSuchTarget_ExitCodeContract runs the real command built
// exactly as root.go wires it (real, un-injected catalog and env) against
// an unknown target name. cliinstall.Plan validates every name against the
// compiled-in catalog BEFORE probing anything
// (cli-install#req:unknown-target-refused: "MUST fail before any
// confirmation, network request or write"), so this is inherently offline
// — no network/env seam is needed to keep it safe for CI.
func TestInstallCmdNoSuchTarget_ExitCodeContract(t *testing.T) {
	cmd := newInstallCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"nosuchcli"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a non-nil error for an unknown install target")
	}
	var coder interface{ ExitCode() int }
	if !errors.As(err, &coder) {
		t.Fatalf("error %v (%T), want an exitcode.Error", err, err)
	}
	if coder.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("ExitCode() = %d, want %d (usage)", coder.ExitCode(), exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "nosuchcli") {
		t.Errorf("error %q does not name the unknown target", err.Error())
	}
}

// TestInstallCmdInvalidFormat_ExitCodeContract proves a plain
// *cobracmd.UsageError (never a KindUnknownTarget failure) still lands in
// cover100's general failure bucket, per task-11's literal two-bucket
// mapping — only KindUnknownTarget gets the usage exit code.
func TestInstallCmdInvalidFormat_ExitCodeContract(t *testing.T) {
	cmd := newInstallCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--format", "yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a non-nil error for --format yaml")
	}
	var coder interface{ ExitCode() int }
	if !errors.As(err, &coder) {
		t.Fatalf("error %v (%T), want an exitcode.Error", err, err)
	}
	if coder.ExitCode() != exitcode.Unexpected {
		t.Errorf("ExitCode() = %d, want %d (failure)", coder.ExitCode(), exitcode.Unexpected)
	}
	if !strings.Contains(err.Error(), "--format") {
		t.Errorf("error %q does not mention --format", err.Error())
	}
}

// TestInstallCmdList_IsOfflineAndSucceeds proves the bare listing form
// (cli-install#req:list-offline-read-only) runs against the real catalog
// and environment without error — it only probes local PATH entries, never
// the network — so it exercises newInstallCmd's wiring end to end once
// more, beyond the unit-level ErrorMapper tests above.
func TestInstallCmdList_IsOfflineAndSucceeds(t *testing.T) {
	cmd := newInstallCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cover100 install (bare listing) failed: %v", err)
	}
	if out.Len() == 0 {
		t.Error("bare listing produced no output")
	}
}

// --- root.go: install subcommand vs. the root command's own [path] arg ---

// TestRootCmd_InstallResolvesToSubcommandNotPathArg proves cobra's own
// command resolution, once "install" is registered, sends `cover100
// install` to the install subcommand while `cover100 ./install` still
// means a path argument for the root collect command — the exact
// distinction task-11 requires ("test that `cover100 install` resolves to
// the subcommand and `cover100 ./install` still means a path").
func TestRootCmd_InstallResolvesToSubcommandNotPathArg(t *testing.T) {
	t.Run("cover100 install resolves to the install subcommand", func(t *testing.T) {
		root, _ := NewRootCmd(fs.FS(nil))
		cmd, _, err := root.Find([]string{"install"})
		if err != nil {
			t.Fatalf("Find([install]) error = %v", err)
		}
		if cmd.Name() != "install" {
			t.Errorf("Find([install]) resolved to %q, want the install subcommand", cmd.Name())
		}
	})

	t.Run("cover100 ./install still means a path argument", func(t *testing.T) {
		root, _ := NewRootCmd(fs.FS(nil))
		cmd, args, err := root.Find([]string{"./install"})
		if err != nil {
			t.Fatalf("Find([./install]) error = %v", err)
		}
		if cmd.Name() != root.Name() {
			t.Errorf("Find([./install]) resolved to %q, want the root command itself", cmd.Name())
		}
		if len(args) != 1 || args[0] != "./install" {
			t.Errorf("Find([./install]) args = %v, want [\"./install\"]", args)
		}
	})
}
