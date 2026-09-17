package cli

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/sneat-dev/cover100-cli/pkg/exitcode"
)

// --- upgrade.go: newUpgradeCmd registration ---

// AC: cli-install#req:core-framework-neutral, cli-install#req:update-alias-
// policy — the command registers the shared upgrade flag surface (no
// --dir) and carries no "update" alias.
func TestNewUpgradeCmd_Registration(t *testing.T) {
	cmd := newUpgradeCmd()
	if !strings.HasPrefix(cmd.Use, "upgrade") {
		t.Errorf("Use = %q, want it to start with upgrade", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short is empty, want a description")
	}
	for _, name := range []string{"all", "check", "yes", "dry-run", "format"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q is not registered", name)
		}
	}
	if cmd.Flags().Lookup("dir") != nil {
		t.Error("upgrade must not register --dir")
	}
	for _, alias := range cmd.Aliases {
		if alias == "update" {
			t.Errorf("upgrade command carries an %q alias; only self-update may keep it", alias)
		}
	}
}

// cli-install#req:host-identity-from-catalog — newUpgradeCmd resolves
// against cover100's own compiled catalog entry without panicking (proven
// by every other test in this file running at all).
func TestNewUpgradeCmd_ResolvesAgainstTheCatalog(t *testing.T) {
	entry, ok := cliinstall.ByID(binaryName)
	if !ok {
		t.Fatalf("no catalog entry for %q", binaryName)
	}
	if entry.ID != binaryName {
		t.Errorf("catalog entry ID = %q, want %q", entry.ID, binaryName)
	}
}

// --- upgrade.go: installErrors reused as upgrade's own error mapper ---

// cli-install#req:unknown-target-refused, cli-install#req:host-owned-exit-
// codes — `cover100 upgrade nosuchcli` MUST fail before any confirmation,
// network request or write, mapped through the SAME installErrors mapper
// install itself uses: exit 2 (usage), naming the unknown target.
func TestUpgradeCmdNoSuchTarget_ExitCodeContract(t *testing.T) {
	cmd := newUpgradeCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"nosuchcli"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a non-nil error for an unknown upgrade target")
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

// TestUpgradeCmdInvalidFormat_ExitCodeContract proves a plain
// *cobracmd.UsageError (never a KindUnknownTarget failure) still lands in
// cover100's usage exit code, matching install's own contract.
func TestUpgradeCmdInvalidFormat_ExitCodeContract(t *testing.T) {
	cmd := newUpgradeCmd()
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
	if coder.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("ExitCode() = %d, want %d (usage)", coder.ExitCode(), exitcode.InvalidArgs)
	}
}

// --- end-to-end wiring: cobracmd.NewUpgrade + selfupdate.Config.Check
// --- against a fake GitHub releases endpoint, proving `self-update` and
// --- `upgrade cover100` reach the same verdict from the SAME HostConfig
// --- (cli-install#req:self-update-equals-upgrade-self,
// --- cli-install#req:host-target-is-running-binary,
// --- cli-install#req:no-network-in-tests). Both commands resolve their
// --- Config through the shared selfUpdateConfigFunc seam, so overriding
// --- that one seam points both at the same fixture server — no network
// --- involved.

func releaseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withFakeReleases(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := selfUpdateConfigFunc
	selfUpdateConfigFunc = func() selfupdate.Config {
		cfg := prev()
		cfg.ReleasesAPIURL = srv.URL
		cfg.HTTPClient = srv.Client()
		return cfg
	}
	t.Cleanup(func() { selfUpdateConfigFunc = prev })
}

// withEmptyUpgradeEnv points upgradeEnv (upgrade.go) at an Env that finds
// nothing installed anywhere: empty PATH, no host dir, nothing executable.
// The bare/--all report otherwise probes every OTHER fleet CLI genuinely
// installed on this real machine's PATH (this VM has specscore, wb and
// codegrapher installed) and makes a REAL release lookup for each one
// found (cli-install#req:upgrade-release-lookups-bounded skips only
// NotInstalled/unrecognized/skipped targets) — this seam is what keeps the
// bare report offline (cli-install#req:no-network-in-tests) without
// depending on this machine's own installed-CLI inventory staying empty.
func withEmptyUpgradeEnv(t *testing.T) {
	t.Helper()
	prev := upgradeEnv
	upgradeEnv = cliinstall.InstallEnv{
		Env: cliinstall.Env{
			PathDirs:     func() []string { return nil },
			HostDir:      func() (string, error) { return "", errors.New("no host dir in test env") },
			IsExecutable: func(string) bool { return false },
			EvalSymlinks: func(p string) (string, error) { return p, nil },
			Run:          func(context.Context, string, []string) ([]byte, error) { return nil, errors.New("not reachable in test env") },
		},
		UserHomeDir: func() (string, error) { return "", errors.New("no home dir in test env") },
		Getenv:      func(string) string { return "" },
		MkdirAll:    func(string, fs.FileMode) error { return errors.New("not writable in test env") },
	}
	t.Cleanup(func() { upgradeEnv = prev })
}

// cli-install#req:upgrade-no-args-reports — the bare report (no names, no
// --all) MUST exit successfully whether or not an upgrade is available,
// exercised offline via the fixture-server seam plus withEmptyUpgradeEnv so
// no OTHER installed fleet CLI's real release lookup can reach the network
// (cli-install#req:no-network-in-tests).
func TestUpgradeCmdReport_OfflineViaFixtureServer(t *testing.T) {
	withFakeReleases(t, releaseServer(t, `[{"tag_name":"v1.0.0","prerelease":false,"draft":false}]`))
	withEmptyUpgradeEnv(t)

	cmd := newUpgradeCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("bare `upgrade` returned error (want exit 0 regardless of verdict): %v", err)
	}
	if out.Len() == 0 {
		t.Error("bare upgrade report produced no output")
	}
}

func TestUpgrade_SelfUpdateEqualsUpgradeSelf(t *testing.T) {
	withFakeReleases(t, releaseServer(t, `[{"tag_name":"v99.0.0","prerelease":false,"draft":false}]`))

	selfCmd := newSelfUpdateCmd()
	var selfOut strings.Builder
	selfCmd.SetOut(&selfOut)
	selfCmd.SetArgs([]string{"--check", "--format", "json"})
	if err := selfCmd.Execute(); err != nil {
		t.Fatalf("self-update --check --format json returned error: %v", err)
	}

	upCmd := newUpgradeCmd()
	var upOut strings.Builder
	upCmd.SetOut(&upOut)
	upCmd.SetArgs([]string{binaryName, "--check", "--format", "json"})
	if err := upCmd.Execute(); err != nil {
		t.Fatalf("upgrade cover100 --check --format json returned error: %v", err)
	}

	if !strings.Contains(selfOut.String(), "99.0.0") {
		t.Fatalf("self-update output %q does not report the fixture latest release", selfOut.String())
	}
	if !strings.Contains(upOut.String(), "99.0.0") {
		t.Errorf("upgrade cover100 output %q does not report the same latest release self-update saw", upOut.String())
	}
}

// TestUpgrade_SelfUpdateEqualsUpgradeSelf_SameFailureSameExitCode is the B1
// fix's own regression test: self-update and upgrade cover100 MUST exit the
// SAME code for the IDENTICAL underlying failure
// (cli-install#req:self-update-equals-upgrade-self: "for every outcome ...
// exit codes stay those self-update already documents"), driven through the
// real commands rather than just the mapper in isolation, now that both
// share installErrors (self_update.go's Errors field).
func TestUpgrade_SelfUpdateEqualsUpgradeSelf_SameFailureSameExitCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	withFakeReleases(t, srv)

	selfCmd := newSelfUpdateCmd()
	selfCmd.SetOut(&strings.Builder{})
	selfCmd.SetArgs([]string{"--check"})
	selfErr := selfCmd.Execute()
	if selfErr == nil {
		t.Fatal("self-update --check against a failing release lookup returned nil, want an error")
	}
	var selfCoder interface{ ExitCode() int }
	if !errors.As(selfErr, &selfCoder) {
		t.Fatalf("self-update error %v (%T) does not expose ExitCode()", selfErr, selfErr)
	}

	upCmd := newUpgradeCmd()
	upCmd.SetOut(&strings.Builder{})
	upCmd.SetArgs([]string{binaryName, "--check"})
	upErr := upCmd.Execute()
	if upErr == nil {
		t.Fatal("upgrade cover100 --check against a failing release lookup returned nil, want an error")
	}
	var upCoder interface{ ExitCode() int }
	if !errors.As(upErr, &upCoder) {
		t.Fatalf("upgrade error %v (%T) does not expose ExitCode()", upErr, upErr)
	}

	if selfCoder.ExitCode() != upCoder.ExitCode() {
		t.Errorf("self-update exit = %d, upgrade cover100 exit = %d; want equal for the identical release-lookup failure", selfCoder.ExitCode(), upCoder.ExitCode())
	}
	if selfCoder.ExitCode() != exitcode.Unexpected {
		t.Errorf("self-update exit = %d, want %d (general failure)", selfCoder.ExitCode(), exitcode.Unexpected)
	}
}

// --- root.go: upgrade subcommand vs. the root command's own [path] arg ---

// TestRootCmd_UpgradeResolvesToSubcommandNotPathArg proves cobra's own
// command resolution, once "upgrade" is registered, sends `cover100
// upgrade` to the upgrade subcommand while `cover100 ./upgrade` still means
// a path argument for the root collect command — the same distinction
// task-11 already requires for `install`
// (cli-install#req:install-verb-vs-path-argument's own reasoning, applied
// to upgrade).
func TestRootCmd_UpgradeResolvesToSubcommandNotPathArg(t *testing.T) {
	t.Run("cover100 upgrade resolves to the upgrade subcommand", func(t *testing.T) {
		root, _ := NewRootCmd(fs.FS(nil))
		cmd, _, err := root.Find([]string{"upgrade"})
		if err != nil {
			t.Fatalf("Find([upgrade]) error = %v", err)
		}
		if cmd.Name() != "upgrade" {
			t.Errorf("Find([upgrade]) resolved to %q, want the upgrade subcommand", cmd.Name())
		}
	})

	t.Run("cover100 ./upgrade still means a path argument", func(t *testing.T) {
		root, _ := NewRootCmd(fs.FS(nil))
		cmd, args, err := root.Find([]string{"./upgrade"})
		if err != nil {
			t.Fatalf("Find([./upgrade]) error = %v", err)
		}
		if cmd.Name() != root.Name() {
			t.Errorf("Find([./upgrade]) resolved to %q, want the root command itself", cmd.Name())
		}
		if len(args) != 1 || args[0] != "./upgrade" {
			t.Errorf("Find([./upgrade]) args = %v, want [\"./upgrade\"]", args)
		}
	})
}
