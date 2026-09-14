// Package aggregate turns per-file collector results into the single
// repository -> package -> file -> function tree the report and the treemap
// consume, computing every node's five coverage metrics.
package aggregate

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/sneat-dev/cover100-cli/internal/model"
)

// ProjectFiles is one detected project's collected files. Several projects may
// share a Rel (a directory holding both a go.mod and a package.json): they
// describe one repository and are merged.
type ProjectFiles struct {
	// Rel is the project directory relative to the scan root, slash-separated.
	Rel   string
	Files []model.FileCoverage
}

// Input is everything Build needs.
type Input struct {
	// Root is the absolute scan root, reported verbatim as the document's root.
	Root string
	// Projects are the collected results, in any order.
	Projects []ProjectFiles
	// Warnings are collector warnings, carried into the report.
	Warnings []string
	// Tool identifies the producing binary and version.
	Tool string
	// Now stamps the document; zero means time.Now().UTC().
	Now time.Time
}

// Build assembles the report. It never returns nil: a scan that found no
// coverage still produces a well-formed document with a childless root.
func Build(in Input) *model.Report {
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	repos := buildRepositories(in.Root, in.Projects)
	root := &model.Node{
		ID:       "root",
		Name:     "root",
		Type:     model.TypeRoot,
		Language: nil,
		Children: repos,
	}
	rollupRoot(root)

	return &model.Report{
		GeneratedAt: now.Format(time.RFC3339),
		Root:        in.Root,
		Metrics:     model.MetricNames,
		Languages:   languagesOf(in.Projects),
		Tree:        root,
		Warnings:    dedupeWarnings(in.Warnings),
		Tool:        in.Tool,
	}
}

// buildRepositories groups projects by directory and builds one node per
// repository, in a deterministic order.
func buildRepositories(scanRoot string, projects []ProjectFiles) []*model.Node {
	type repoGroup struct {
		rel   string
		files []model.FileCoverage
	}
	groups := make(map[string]*repoGroup)
	var order []string
	for _, p := range projects {
		rel := p.Rel
		if rel == "" {
			rel = "."
		}
		g := groups[rel]
		if g == nil {
			g = &repoGroup{rel: rel}
			groups[rel] = g
			order = append(order, rel)
		}
		g.files = append(g.files, p.Files...)
	}
	sort.Strings(order)

	base := path.Base(strings.ReplaceAll(scanRoot, "\\", "/"))
	nodes := make([]*model.Node, 0, len(order))
	for _, rel := range order {
		g := groups[rel]
		name := base
		if rel != "." && rel != "" {
			name = strings.ReplaceAll(rel, "/", "-")
		}
		language := languageOfFiles(g.files)
		node := &model.Node{
			ID:       "repo:" + rel,
			Name:     name,
			Type:     model.TypeRepository,
			Language: model.StrPtr(language),
			Path:     rel,
			Children: buildPackages(rel, g.files),
		}
		rollupRepository(node)
		nodes = append(nodes, node)
	}
	sortNodes(nodes)
	return nodes
}

// buildPackages groups a repository's files by directory and builds the
// package nodes, each carrying its file nodes.
func buildPackages(repoRel string, files []model.FileCoverage) []*model.Node {
	byPkg := make(map[string][]model.FileCoverage)
	var order []string
	for _, f := range files {
		dir := path.Dir(f.Path)
		if _, ok := byPkg[dir]; !ok {
			order = append(order, dir)
		}
		byPkg[dir] = append(byPkg[dir], f)
	}
	sort.Strings(order)

	nodes := make([]*model.Node, 0, len(order))
	for _, dir := range order {
		pkgFiles := byPkg[dir]
		sort.Slice(pkgFiles, func(i, j int) bool { return pkgFiles[i].Path < pkgFiles[j].Path })

		childNodes := make([]*model.Node, 0, len(pkgFiles))
		for i := range pkgFiles {
			childNodes = append(childNodes, buildFile(repoRel, &pkgFiles[i]))
		}

		name := path.Base(dir)
		relToRepo := relToRepo(repoRel, dir)
		if relToRepo == "." {
			name = "(root)"
		}
		node := &model.Node{
			ID:       "pkg:" + dir,
			Name:     name,
			Type:     model.TypePackage,
			Language: model.StrPtr(languageOfFiles(pkgFiles)),
			Path:     relToRepo,
			Children: childNodes,
		}
		rollupPackage(node)
		sortNodes(node.Children)
		nodes = append(nodes, node)
	}
	sortNodes(nodes)
	return nodes
}

// buildFile builds a file node and its function children.
func buildFile(repoRel string, f *model.FileCoverage) *model.Node {
	node := &model.Node{
		ID:       "file:" + f.Path,
		Name:     path.Base(f.Path),
		Type:     model.TypeFile,
		Language: model.StrPtr(f.Language),
		Path:     relToRepo(repoRel, f.Path),
	}
	node.Lines = f.Lines
	node.Functions = f.Functions
	if !f.FunctionsAvailable {
		node.FunctionsAvailable = model.BoolPtr(false)
	}
	if f.Branches != nil {
		branch := *f.Branches
		node.Branches = &branch
	}

	if f.FunctionsAvailable {
		for i := range f.FunctionList {
			node.Children = append(node.Children, buildFunction(f.Path, &f.FunctionList[i]))
		}
		sortNodes(node.Children)
	}

	// A file's own line and function numbers are authoritative: they come from
	// the file's full statement and function tables, whereas its function
	// children only own the statements inside a function body.
	covered := f.Lines.Any()
	node.Files = model.Metric{Covered: covered, Total: 1}
	node.Packages = model.Metric{Covered: covered, Total: 1}
	node.Repositories = model.Metric{Covered: covered, Total: 1}
	return node
}

// buildFunction builds a leaf function node.
func buildFunction(filePath string, fc *model.FuncCoverage) *model.Node {
	line := fc.Line
	hits := fc.Hits
	covered := fc.Covered
	node := &model.Node{
		ID:       fmt.Sprintf("fn:%s#%s@%d", filePath, fc.Name, fc.Line),
		Name:     fc.Name,
		Type:     model.TypeFunction,
		Language: nil,
		Line:     &line,
		Hits:     &hits,
		Covered:  &covered,
	}
	node.Lines = fc.Lines
	node.Functions = model.Metric{Covered: boolToInt(covered), Total: 1}
	anyLine := fc.Lines.Any()
	node.Files = model.Metric{Covered: anyLine, Total: 1}
	node.Packages = model.Metric{Covered: anyLine, Total: 1}
	node.Repositories = model.Metric{Covered: anyLine, Total: 1}
	return node
}

// rollupPackage sums its file children and records the package's own identity
// in the packages/repositories dimensions.
func rollupPackage(node *model.Node) {
	for _, child := range node.Children {
		node.Lines = node.Lines.Add(child.Lines)
		node.Functions = node.Functions.Add(child.Functions)
		node.Files = node.Files.Add(child.Files)
	}
	covered := node.Lines.Any()
	node.Packages = model.Metric{Covered: covered, Total: 1}
	node.Repositories = model.Metric{Covered: covered, Total: 1}
}

// rollupRepository sums its package children; packages counts how many of them
// contain any covered line, which is the number a reader expects to see.
func rollupRepository(node *model.Node) {
	for _, child := range node.Children {
		node.Lines = node.Lines.Add(child.Lines)
		node.Functions = node.Functions.Add(child.Functions)
		node.Files = node.Files.Add(child.Files)
		node.Packages = node.Packages.Add(model.Metric{Covered: child.Lines.Any(), Total: 1})
	}
	node.Repositories = model.Metric{Covered: node.Lines.Any(), Total: 1}
}

// rollupRoot aggregates repositories and counts how many of them have any
// coverage at all.
func rollupRoot(node *model.Node) {
	for _, child := range node.Children {
		node.Lines = node.Lines.Add(child.Lines)
		node.Functions = node.Functions.Add(child.Functions)
		node.Files = node.Files.Add(child.Files)
		node.Packages = node.Packages.Add(child.Packages)
		node.Repositories = node.Repositories.Add(model.Metric{Covered: child.Lines.Any(), Total: 1})
	}
}

// sortNodes orders children by size so the treemap's reading order is stable:
// the largest by measured lines first, then by functions, then by name.
func sortNodes(nodes []*model.Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.Lines.Total != b.Lines.Total {
			return a.Lines.Total > b.Lines.Total
		}
		if a.Functions.Total != b.Functions.Total {
			return a.Functions.Total > b.Functions.Total
		}
		return a.Name < b.Name
	})
}

// languageOfFiles classifies a repository or package by the files it holds.
// TypeScript wins over JavaScript because a mixed JS/TS package is a
// TypeScript package with some plain JavaScript in it.
func languageOfFiles(files []model.FileCoverage) string {
	var hasGo, hasTS, hasJS bool
	for _, f := range files {
		switch f.Language {
		case model.LangGo:
			hasGo = true
		case model.LangTypeScript:
			hasTS = true
		case model.LangJavaScript:
			hasJS = true
		}
	}
	switch {
	case hasGo && (hasTS || hasJS):
		return model.LangMixed
	case hasGo:
		return model.LangGo
	case hasTS:
		return model.LangTypeScript
	case hasJS:
		return model.LangJavaScript
	default:
		return model.LangMixed
	}
}

// languagesOf reports the languages actually present, in canonical order, so
// the UI's language filter is built from data rather than from a fixed list.
func languagesOf(projects []ProjectFiles) []string {
	seen := map[string]bool{}
	for _, p := range projects {
		for _, f := range p.Files {
			seen[f.Language] = true
		}
	}
	var out []string
	for _, lang := range []string{model.LangGo, model.LangTypeScript, model.LangJavaScript} {
		if seen[lang] {
			out = append(out, lang)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// relToRepo converts a scan-root-relative path into a repository-relative one.
func relToRepo(repoRel, scanRel string) string {
	if repoRel == "." || repoRel == "" {
		return scanRel
	}
	if scanRel == repoRel {
		return "."
	}
	if rest, ok := strings.CutPrefix(scanRel, repoRel+"/"); ok {
		return rest
	}
	return scanRel
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// dedupeWarnings keeps the first occurrence of each warning and preserves
// order, so a warning repeated once per project is reported once.
func dedupeWarnings(warnings []string) []string {
	seen := make(map[string]bool, len(warnings))
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
