package aggregate

import (
	"testing"
	"time"

	"github.com/sneat-dev/cover100-cli/internal/model"
)

func file(path, language string, covered, total int, funcs ...model.FuncCoverage) model.FileCoverage {
	coveredFuncs := 0
	for _, f := range funcs {
		if f.Covered {
			coveredFuncs++
		}
	}
	return model.FileCoverage{
		Path:               path,
		Language:           language,
		Lines:              model.Metric{Covered: covered, Total: total},
		Functions:          model.Metric{Covered: coveredFuncs, Total: len(funcs)},
		FunctionsAvailable: len(funcs) > 0,
		FunctionList:       funcs,
	}
}

func TestBuild_RollsUpEveryMetricDimension(t *testing.T) {
	report := Build(Input{
		Root: "/work/my-app",
		Tool: "cover100 test",
		Now:  time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		Projects: []ProjectFiles{
			{Rel: ".", Files: []model.FileCoverage{
				file("src/checkout/cart.ts", model.LangTypeScript, 8, 10,
					model.FuncCoverage{Name: "addItem", Line: 4, Hits: 2, Covered: true,
						Lines: model.Metric{Covered: 3, Total: 3}},
					model.FuncCoverage{Name: "removeItem", Line: 9, Hits: 0,
						Lines: model.Metric{Covered: 0, Total: 2}},
				),
				file("src/checkout/empty.ts", model.LangTypeScript, 0, 5),
				file("src/util/format.ts", model.LangTypeScript, 5, 5),
			}},
			{Rel: "cmd/tool", Files: []model.FileCoverage{
				file("cmd/tool/main.go", model.LangGo, 0, 4),
			}},
		},
	})

	if report.GeneratedAt != "2026-09-14T12:00:00Z" {
		t.Errorf("GeneratedAt = %q", report.GeneratedAt)
	}
	wantLangs := []string{model.LangGo, model.LangTypeScript}
	if len(report.Languages) != 2 || report.Languages[0] != wantLangs[0] || report.Languages[1] != wantLangs[1] {
		t.Errorf("Languages = %v, want %v", report.Languages, wantLangs)
	}

	root := report.Tree
	if want := (model.Metric{Covered: 13, Total: 24}); root.Lines != want {
		t.Errorf("root.Lines = %+v, want %+v", root.Lines, want)
	}
	if want := (model.Metric{Covered: 1, Total: 2}); root.Functions != want {
		t.Errorf("root.Functions = %+v, want %+v", root.Functions, want)
	}
	if want := (model.Metric{Covered: 2, Total: 4}); root.Files != want {
		t.Errorf("root.Files = %+v, want %+v", root.Files, want)
	}
	// Three packages: src/checkout, src/util, cmd/tool. The last has no covered
	// lines, so packages reports 2 of 3.
	if want := (model.Metric{Covered: 2, Total: 3}); root.Packages != want {
		t.Errorf("root.Packages = %+v, want %+v", root.Packages, want)
	}
	// Two repositories, one of which has no covered lines.
	if want := (model.Metric{Covered: 1, Total: 2}); root.Repositories != want {
		t.Errorf("root.Repositories = %+v, want %+v", root.Repositories, want)
	}
}

func TestBuild_NodeIDsAreUniqueAndStable(t *testing.T) {
	report := Build(Input{
		Root: "/work/app",
		Projects: []ProjectFiles{
			{Rel: ".", Files: []model.FileCoverage{
				file("src/cart.ts", model.LangTypeScript, 1, 1,
					model.FuncCoverage{Name: "add", Line: 7, Hits: 1, Covered: true,
						Lines: model.Metric{Covered: 1, Total: 1}}),
			}},
		},
	})

	seen := map[string]bool{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if seen[n.ID] {
			t.Errorf("duplicate node id %q", n.ID)
		}
		seen[n.ID] = true
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(report.Tree)

	for _, want := range []string{
		"root",
		"repo:.",
		"pkg:src",
		"file:src/cart.ts",
		"fn:src/cart.ts#add@7",
	} {
		if !seen[want] {
			t.Errorf("missing node id %q (have %v)", want, keys(seen))
		}
	}
}

func TestBuild_GoFilesReportFunctionsUnavailable(t *testing.T) {
	report := Build(Input{
		Root: "/work/app",
		Projects: []ProjectFiles{
			{Rel: ".", Files: []model.FileCoverage{
				{Path: "main.go", Language: model.LangGo,
					Lines: model.Metric{Covered: 1, Total: 2}, FunctionsAvailable: false},
			}},
		},
	})

	fileNode := report.Tree.Children[0].Children[0].Children[0]
	if fileNode.Type != model.TypeFile {
		t.Fatalf("expected a file node, got %s", fileNode.Type)
	}
	if fileNode.FunctionsAvailable == nil || *fileNode.FunctionsAvailable {
		t.Error("Go file must be marked functionsAvailable: false")
	}
	if fileNode.Functions.Total != 0 {
		t.Errorf("Go file functions = %+v, want 0/0", fileNode.Functions)
	}
}

func TestBuild_ChildrenSortedByLinesTotalDescending(t *testing.T) {
	report := Build(Input{
		Root: "/work/app",
		Projects: []ProjectFiles{
			{Rel: ".", Files: []model.FileCoverage{
				file("small.ts", model.LangTypeScript, 1, 2),
				file("big.ts", model.LangTypeScript, 1, 90),
				file("medium.ts", model.LangTypeScript, 1, 20),
			}},
		},
	})

	var names []string
	for _, child := range report.Tree.Children[0].Children[0].Children {
		names = append(names, child.Name)
	}
	want := []string{"big.ts", "medium.ts", "small.ts"}
	for i := range want {
		if i >= len(names) || names[i] != want[i] {
			t.Fatalf("child order = %v, want %v", names, want)
		}
	}
}

func TestBuild_MergesGoAndNodeProjectsInOneDirectory(t *testing.T) {
	report := Build(Input{
		Root: "/work/app",
		Projects: []ProjectFiles{
			{Rel: ".", Files: []model.FileCoverage{file("main.go", model.LangGo, 1, 1)}},
			{Rel: ".", Files: []model.FileCoverage{file("src/a.ts", model.LangTypeScript, 1, 1)}},
		},
	})

	if len(report.Tree.Children) != 1 {
		t.Fatalf("got %d repositories, want 1 merged repository", len(report.Tree.Children))
	}
	repo := report.Tree.Children[0]
	if repo.Language == nil || *repo.Language != model.LangMixed {
		t.Errorf("repository language = %v, want mixed", repo.Language)
	}
	if want := (model.Metric{Covered: 2, Total: 2}); repo.Files != want {
		t.Errorf("repository Files = %+v, want %+v", repo.Files, want)
	}
}

func TestBuild_EmptyInputStillProducesAValidRoot(t *testing.T) {
	report := Build(Input{Root: "/work/empty"})
	if report.Tree == nil || report.Tree.Type != model.TypeRoot {
		t.Fatal("Build must always return a root node")
	}
	if len(report.Tree.Children) != 0 {
		t.Errorf("children = %v, want none", report.Tree.Children)
	}
	if report.Languages == nil {
		t.Error("Languages must be an empty slice, not nil, so it serializes as []")
	}
}

func TestPackageAndRepositoryPathsAreRelativeToTheRepository(t *testing.T) {
	report := Build(Input{
		Root: "/work/mono",
		Projects: []ProjectFiles{
			{Rel: "services/api", Files: []model.FileCoverage{
				file("services/api/internal/store/store.go", model.LangGo, 1, 1),
			}},
		},
	})

	repo := report.Tree.Children[0]
	if repo.Name != "services-api" {
		t.Errorf("repository name = %q, want services-api", repo.Name)
	}
	pkg := repo.Children[0]
	if pkg.Path != "internal/store" {
		t.Errorf("package path = %q, want internal/store", pkg.Path)
	}
	fileNode := pkg.Children[0]
	if fileNode.Path != "internal/store/store.go" {
		t.Errorf("file path = %q, want internal/store/store.go", fileNode.Path)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
