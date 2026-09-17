package cli

import (
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall"
)

// cli-install#req:host-identity-from-catalog — a host id absent from the
// compiled catalog is a programming error caught by this package's own
// tests, never a runtime state a user can trigger. Before the M5 fix,
// selfUpdateConfig ignored cliinstall.ByID's not-found case
// (`entry, _ := cliinstall.ByID(...)`) and would have silently run
// self-update with an empty selfupdate.Config instead, matching neither
// specscore's nor chatwright's own panic.
func TestSelfUpdateConfig_PanicsWhenCatalogEntryMissing(t *testing.T) {
	prev := catalogEntryByID
	catalogEntryByID = func(string) (cliinstall.Entry, bool) { return cliinstall.Entry{}, false }
	t.Cleanup(func() { catalogEntryByID = prev })

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected selfUpdateConfig to panic when the catalog entry is missing")
		}
	}()
	selfUpdateConfig()
}

// cli-install#req:catalog-identity-single-source — selfUpdateConfig resolves
// cover100's release identity from the SAME compiled-in catalog entry
// install/upgrade resolve, not a hand-maintained duplicate.
func TestSelfUpdateConfig_MatchesCatalogEntry(t *testing.T) {
	entry, ok := cliinstall.ByID(binaryName)
	if !ok {
		t.Fatalf("no catalog entry for %q", binaryName)
	}
	want := entry.Config(buildInfo.Version)
	got := selfUpdateConfig()
	if got.BinaryName != want.BinaryName || got.Repository != want.Repository {
		t.Errorf("selfUpdateConfig() = %+v, want built from cliinstall.ByID(%q).Config(...): %+v", got, binaryName, want)
	}
}

// TestNewSelfUpdateCmd_UsesSharedErrorMapper proves self-update's own
// Errors field is installErrors{} -- the SAME value install.go and
// upgrade.go use -- rather than a bespoke passthrough (the B1 fix: before
// it, self-update let every error fall through to root.go's Fatal default
// of exit 1, which disagreed with upgrade cover100's exit 10 for the
// identical failure).
func TestNewSelfUpdateCmd_UsesSharedErrorMapper(t *testing.T) {
	cmd := newSelfUpdateCmd()
	if cmd.Name() != "self-update" {
		t.Errorf("Name() = %q, want self-update", cmd.Name())
	}
	if !strings.Contains(strings.Join(cmd.Aliases, ","), "update") {
		t.Error("missing update alias")
	}
}
