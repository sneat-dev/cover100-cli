package aggregate

import (
	"testing"

	"github.com/sneat-dev/cover100-cli/internal/model"
)

// TestBuild_DefaultRelNamesRepositoryAfterScanRoot covers a collector that
// reports an empty Rel: that means "the scan root itself", so the repository
// must be identified as "." and named after the root's base name.
func TestBuild_DefaultRelNamesRepositoryAfterScanRoot(t *testing.T) {
	branches := model.Metric{Covered: 3, Total: 4}
	report := Build(Input{
		Root: "/work/my-app",
		Projects: []ProjectFiles{
			{Rel: "", Files: []model.FileCoverage{
				{
					Path:     "src/a.ts",
					Language: model.LangTypeScript,
					Lines:    model.Metric{Covered: 1, Total: 2},
					Branches: &branches,
				},
			}},
		},
	})

	if len(report.Tree.Children) != 1 {
		t.Fatalf("repositories = %d, want 1", len(report.Tree.Children))
	}
	repo := report.Tree.Children[0]
	if repo.ID != "repo:." || repo.Path != "." {
		t.Errorf("repository identity = (%q, %q), want (\"repo:.\", \".\"): an empty project Rel means the scan root",
			repo.ID, repo.Path)
	}
	if repo.Name != "my-app" {
		t.Errorf("repository name = %q, want my-app (the scan root's base name)", repo.Name)
	}

	fileNode := repo.Children[0].Children[0]
	if fileNode.Branches == nil {
		t.Fatal("file node Branches = nil, want the collector's branch metric carried through")
	}
	if *fileNode.Branches != branches {
		t.Errorf("file node Branches = %+v, want %+v", *fileNode.Branches, branches)
	}
}

// TestBuild_SortsSiblingsByFunctionsThenName pins both tie-breakers in
// sortNodes: equal line totals fall back to function totals, and fully equal
// metrics fall back to the node name so the treemap's reading order is stable.
func TestBuild_SortsSiblingsByFunctionsThenName(t *testing.T) {
	report := Build(Input{
		Root: "/work/app",
		Projects: []ProjectFiles{
			{Rel: ".", Files: []model.FileCoverage{
				file("pkg/zeta.ts", model.LangTypeScript, 1, 5),
				file("pkg/alpha.ts", model.LangTypeScript, 1, 5),
				file("pkg/middle.ts", model.LangTypeScript, 1, 5,
					model.FuncCoverage{Name: "wrapped", Line: 1, Hits: 1, Covered: true,
						Lines: model.Metric{Covered: 1, Total: 1}},
					model.FuncCoverage{Name: "skipped", Line: 5, Hits: 0,
						Lines: model.Metric{Covered: 0, Total: 1}},
				),
			}},
		},
	})

	pkg := report.Tree.Children[0].Children[0]
	var names []string
	for _, child := range pkg.Children {
		names = append(names, child.Name)
	}
	want := []string{"middle.ts", "alpha.ts", "zeta.ts"}
	if len(names) != len(want) {
		t.Fatalf("file children = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("file children = %v, want %v (functions.total then name breaks ties)", names, want)
		}
	}
}

func TestLanguageOfFiles(t *testing.T) {
	goFile := model.FileCoverage{Language: model.LangGo}
	tsFile := model.FileCoverage{Language: model.LangTypeScript}
	jsFile := model.FileCoverage{Language: model.LangJavaScript}

	tests := []struct {
		name  string
		files []model.FileCoverage
		want  string
	}{
		{"go only", []model.FileCoverage{goFile, goFile}, model.LangGo},
		{"typescript only", []model.FileCoverage{tsFile}, model.LangTypeScript},
		{"javascript only", []model.FileCoverage{jsFile}, model.LangJavaScript},
		{"typescript wins over javascript", []model.FileCoverage{jsFile, tsFile}, model.LangTypeScript},
		{"go with typescript is mixed", []model.FileCoverage{goFile, tsFile}, model.LangMixed},
		{"go with javascript is mixed", []model.FileCoverage{goFile, jsFile}, model.LangMixed},
		{"no files is mixed", nil, model.LangMixed},
		{"empty slice is mixed", []model.FileCoverage{}, model.LangMixed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := languageOfFiles(tc.files); got != tc.want {
				t.Errorf("languageOfFiles(%v) = %q, want %q", tc.files, got, tc.want)
			}
		})
	}
}

func TestRelToRepo(t *testing.T) {
	tests := []struct {
		name             string
		repoRel, scanRel string
		want             string
	}{
		{"scan root repository", ".", "src/a.ts", "src/a.ts"},
		{"empty repository rel", "", "src/a.ts", "src/a.ts"},
		{"path is the repository directory", "services/api", "services/api", "."},
		{"path below the repository", "services/api", "services/api/internal/store.go", "internal/store.go"},
		{"path outside the repository", "services/api", "cmd/tool/main.go", "cmd/tool/main.go"},
		{"shared text prefix is not a path prefix", "services/api", "services/api2/x.go", "services/api2/x.go"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relToRepo(tc.repoRel, tc.scanRel); got != tc.want {
				t.Errorf("relToRepo(%q, %q) = %q, want %q", tc.repoRel, tc.scanRel, got, tc.want)
			}
		})
	}
}

func TestDedupeWarnings(t *testing.T) {
	got := dedupeWarnings([]string{"", "first", "first", "second", "", "second", "third"})
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("dedupeWarnings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupeWarnings = %v, want %v (first occurrence wins, order preserved)", got, want)
		}
	}

	if got := dedupeWarnings([]string{"", ""}); got != nil {
		t.Errorf("dedupeWarnings of only empty strings = %v, want nil", got)
	}
	if got := dedupeWarnings(nil); got != nil {
		t.Errorf("dedupeWarnings(nil) = %v, want nil", got)
	}
}

func TestBuild_CarriesDedupedWarnings(t *testing.T) {
	report := Build(Input{Root: "/work/app", Warnings: []string{"same", "same", ""}})
	if len(report.Warnings) != 1 || report.Warnings[0] != "same" {
		t.Errorf("report.Warnings = %v, want [\"same\"]", report.Warnings)
	}
}
