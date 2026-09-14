// Package detect finds the coverage collection targets under a directory:
// Go modules (a go.mod file) and Node packages (a package.json file).
package detect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind identifies the ecosystem of a detected project.
type Kind string

const (
	KindGo   Kind = "go"
	KindNode Kind = "node"
)

// skipDirs are never descended into: they are either VCS metadata, dependency
// trees that would otherwise dominate the walk, or cover100's own scratch
// space.
var skipDirs = map[string]bool{
	"node_modules":     true,
	"vendor":           true,
	"bower_components": true,
}

// Project is one coverage collection target.
type Project struct {
	// Dir is the absolute directory holding the manifest.
	Dir string
	// Rel is Dir relative to the scan root, slash-separated; "." for the root
	// itself.
	Rel  string
	Kind Kind

	// Go module path, read from the go.mod module directive.
	ModulePath string

	// Node package metadata.
	PackageName   string
	TestScript    string
	HasTestScript bool
	// Runner is "vitest", "jest" or "" when no known runner could be inferred.
	Runner string
	// HasNodeModules reports whether a node_modules directory sits beside the
	// manifest, which determines whether a local runner binary can be used.
	HasNodeModules bool
}

// Result is the outcome of a scan.
type Result struct {
	Root     string
	Projects []Project
	// HasTypeScript and HasJavaScript record whether the tree contains files of
	// those kinds at all, independent of whether any of them are runnable.
	HasTypeScript bool
	HasJavaScript bool
}

// GoModules returns the detected Go projects.
func (r *Result) GoModules() []Project {
	return r.filter(KindGo)
}

// NodePackages returns the detected Node projects that declare a test script.
func (r *Result) NodePackages() []Project {
	var out []Project
	for _, p := range r.Projects {
		if p.Kind == KindNode && p.HasTestScript {
			out = append(out, p)
		}
	}
	return out
}

func (r *Result) filter(k Kind) []Project {
	var out []Project
	for _, p := range r.Projects {
		if p.Kind == k {
			out = append(out, p)
		}
	}
	return out
}

// packageJSON is the subset of package.json cover100 reads.
type packageJSON struct {
	Name            string            `json:"name"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// Scan walks root and returns every Go module and Node package it finds.
//
// Hidden directories are skipped (which covers .git, .cover100 and editor
// state), as are node_modules and vendor. Unreadable subtrees are skipped
// rather than failing the scan: a permission error deep in a tree should not
// prevent collecting coverage from everything else.
func Scan(root string) (*Result, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}

	res := &Result{Root: abs}
	walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == abs {
				return err
			}
			// Below the root, WalkDir reports an error only for a directory it
			// could not read — it never stats a plain file — so skipping that
			// subtree is exactly right: one unreadable directory must not
			// abandon the rest of the tree.
			return fs.SkipDir
		}
		if d.IsDir() {
			if p != abs && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()]) {
				return fs.SkipDir
			}
			return nil
		}

		dir := filepath.Dir(p)
		switch d.Name() {
		case "go.mod":
			project := Project{Dir: dir, Rel: relSlash(abs, dir), Kind: KindGo}
			if project.ModulePath, err = readModulePath(p); err != nil {
				return nil
			}
			res.Projects = append(res.Projects, project)
		case "package.json":
			pkg, err := readPackageJSON(p)
			if err != nil {
				return nil
			}
			_, statErr := os.Stat(filepath.Join(dir, "node_modules"))
			res.Projects = append(res.Projects, Project{
				Dir:            dir,
				Rel:            relSlash(abs, dir),
				Kind:           KindNode,
				PackageName:    pkg.Name,
				TestScript:     pkg.Scripts["test"],
				HasTestScript:  strings.TrimSpace(pkg.Scripts["test"]) != "",
				Runner:         detectRunner(pkg),
				HasNodeModules: statErr == nil,
			})
		case "tsconfig.json", "jsconfig.json":
			res.HasTypeScript = true
		}

		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".ts", ".tsx", ".mts", ".cts":
			res.HasTypeScript = true
		case ".js", ".jsx", ".mjs", ".cjs":
			res.HasJavaScript = true
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Deterministic order keeps concurrent collection and the resulting report
	// reproducible across runs.
	sort.Slice(res.Projects, func(i, j int) bool {
		if res.Projects[i].Dir != res.Projects[j].Dir {
			return res.Projects[i].Dir < res.Projects[j].Dir
		}
		return res.Projects[i].Kind < res.Projects[j].Kind
	})
	return res, nil
}

// readModulePath extracts the module path from a go.mod file.
func readModulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module"); ok {
			rest = strings.TrimSpace(rest)
			rest = strings.Trim(rest, `"`)
			if rest != "" {
				return rest, nil
			}
		}
	}
	return "", errors.New("no module directive")
}

func readPackageJSON(path string) (packageJSON, error) {
	var pkg packageJSON
	data, err := os.ReadFile(path)
	if err != nil {
		return pkg, err
	}
	if err = json.Unmarshal(data, &pkg); err != nil {
		return pkg, fmt.Errorf("parse %s: %w", path, err)
	}
	return pkg, nil
}

// detectRunner infers the test runner. Declared dependencies win over the test
// script's text, because a script like "npm run build && node test.js" may
// mention neither while a dependency is authoritative.
func detectRunner(pkg packageJSON) string {
	for _, runner := range []string{"vitest", "jest"} {
		if _, ok := pkg.DevDependencies[runner]; ok {
			return runner
		}
		if _, ok := pkg.Dependencies[runner]; ok {
			return runner
		}
	}
	script := strings.ToLower(pkg.Scripts["test"])
	if strings.Contains(script, "vitest") {
		return "vitest"
	}
	if strings.Contains(script, "jest") {
		return "jest"
	}
	return ""
}

func relSlash(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "" {
		return "."
	}
	return filepath.ToSlash(rel)
}
