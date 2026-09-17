package cli

import (
	"os"
	"sort"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/selfupdate"
	"gopkg.in/yaml.v3"
)

// goreleaserConfig decodes only the .goreleaser.yml fields this consumer
// test needs to compare against the compiled-in catalog entry
// (cli-install#req:catalog-identity-single-source): archive/checksums
// naming, the release repository, and the supported platform matrix. It is
// deliberately not a full GoReleaser schema.
type goreleaserConfig struct {
	ProjectName string `yaml:"project_name"`
	Builds      []struct {
		GOOS   []string `yaml:"goos"`
		GOARCH []string `yaml:"goarch"`
		Ignore []struct {
			GOOS   string `yaml:"goos"`
			GOARCH string `yaml:"goarch"`
		} `yaml:"ignore"`
	} `yaml:"builds"`
	Archives []struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	Release struct {
		GitHub struct {
			Owner string `yaml:"owner"`
			Name  string `yaml:"name"`
		} `yaml:"github"`
	} `yaml:"release"`
	HomebrewCasks []struct {
		Name string `yaml:"name"`
	} `yaml:"homebrew_casks"`
}

// AC: cli-install#ac:catalog-matrix-is-valid — a consumer-side offline test
// asserting that cover100's own .goreleaser.yml archive name template,
// checksum name template, platforms, release repository and tag prefix
// match its cli-helpers catalog entry, so drift fails cover100's own CI
// rather than surfacing as a 404 in someone else's `install cover100`
// (cli-install#req:catalog-identity-single-source).
func TestCover100CatalogEntry_MatchesGoReleaserConfig(t *testing.T) {
	entry, ok := cliinstall.ByID(binaryName)
	if !ok {
		t.Fatalf("no catalog entry for %q", binaryName)
	}

	raw, err := os.ReadFile("../../.goreleaser.yml")
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}
	var gr goreleaserConfig
	if err := yaml.Unmarshal(raw, &gr); err != nil {
		t.Fatalf("parse .goreleaser.yml: %v", err)
	}

	if gr.ProjectName != entry.ID {
		t.Errorf("goreleaser project_name = %q, want catalog id %q", gr.ProjectName, entry.ID)
	}
	if got, want := gr.Release.GitHub.Owner+"/"+gr.Release.GitHub.Name, entry.Repository; got != want {
		t.Errorf("goreleaser release.github = %q, want catalog Repository %q", got, want)
	}
	if entry.TagPrefix != "" {
		t.Errorf("catalog TagPrefix = %q, want empty: cover100 publishes one product per repository", entry.TagPrefix)
	}

	// checksum: the catalog entry leaves ChecksumsName nil, which means
	// "use the library's own GoReleaser-shaped default"
	// ("<binary>_<version>_checksums.txt") — verify .goreleaser.yml's own
	// template is exactly that shape (with the literal binary name, which
	// equals the project name here) so the two can never silently diverge.
	if len(gr.Archives) == 0 {
		t.Fatal(".goreleaser.yml declares no archives")
	}
	wantChecksumTemplate := entry.ID + "_{{ .Version }}_checksums.txt"
	if gr.Checksum.NameTemplate != wantChecksumTemplate {
		t.Errorf(".goreleaser.yml checksum.name_template = %q, want %q", gr.Checksum.NameTemplate, wantChecksumTemplate)
	}
	if entry.ChecksumsName != nil {
		t.Error("catalog entry.ChecksumsName is overridden; want nil (the shared GoReleaser default matches .goreleaser.yml)")
	}

	// archive naming: the catalog entry leaves AssetName nil too, so
	// .goreleaser.yml's own archive template must match the library's
	// default shape exactly.
	wantArchiveTemplate := entry.ID + "_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
	if gr.Archives[0].NameTemplate != wantArchiveTemplate {
		t.Errorf(".goreleaser.yml archives[0].name_template = %q, want %q (the library's default asset-naming shape)",
			gr.Archives[0].NameTemplate, wantArchiveTemplate)
	}
	if entry.AssetName != nil {
		t.Error("catalog entry.AssetName is overridden; want nil (the shared GoReleaser default matches .goreleaser.yml)")
	}

	// platform matrix: builds[].goos x goarch, minus builds[].ignore, must
	// equal entry.SupportedPlatforms exactly (order-independent).
	if len(gr.Builds) == 0 {
		t.Fatal(".goreleaser.yml declares no builds")
	}
	build := gr.Builds[0]
	ignored := make(map[[2]string]bool, len(build.Ignore))
	for _, ig := range build.Ignore {
		ignored[[2]string{ig.GOOS, ig.GOARCH}] = true
	}
	var goreleaserPlatforms []selfupdate.Platform
	for _, goos := range build.GOOS {
		for _, goarch := range build.GOARCH {
			if ignored[[2]string{goos, goarch}] {
				continue
			}
			goreleaserPlatforms = append(goreleaserPlatforms, selfupdate.Platform{GOOS: goos, GOARCH: goarch})
		}
	}
	if got, want := sortedPlatforms(goreleaserPlatforms), sortedPlatforms(entry.SupportedPlatforms); !platformsEqual(got, want) {
		t.Errorf("goreleaser platform matrix = %v, want catalog entry.SupportedPlatforms %v", got, want)
	}

	// cover100 publishes no Homebrew cask: neither .goreleaser.yml nor the
	// catalog entry should declare one.
	if len(gr.HomebrewCasks) != 0 {
		t.Errorf("goreleaser declares %d homebrew_casks, want 0 (catalog entry.CaskToken is empty)", len(gr.HomebrewCasks))
	}
	if entry.CaskToken != "" {
		t.Errorf("catalog CaskToken = %q, want empty: .goreleaser.yml declares no homebrew_casks", entry.CaskToken)
	}
}

func sortedPlatforms(in []selfupdate.Platform) []selfupdate.Platform {
	out := append([]selfupdate.Platform(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].GOOS != out[j].GOOS {
			return out[i].GOOS < out[j].GOOS
		}
		return out[i].GOARCH < out[j].GOARCH
	})
	return out
}

func platformsEqual(a, b []selfupdate.Platform) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
