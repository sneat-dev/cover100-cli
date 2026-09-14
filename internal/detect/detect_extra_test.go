package detect

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestScan_SkipsAnUnreadableSubtreeAndKeepsScanning covers the walk error
// handler: a permission error deep in a tree must cost only that subtree, not
// the whole scan.
func TestScan_SkipsAnUnreadableSubtreeAndKeepsScanning(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n")
	writeFile(t, filepath.Join(root, "locked", "package.json"),
		`{"name":"locked","scripts":{"test":"jest"}}`)
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatalf("chmod %s: %v", locked, err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("permission bits are not enforced (running as root?); cannot exercise an unreadable subtree")
	}

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v; an unreadable subtree must be skipped, not fail the scan", err)
	}
	if len(result.Projects) != 1 || result.Projects[0].Kind != KindGo {
		t.Errorf("Projects = %+v, want only the readable root module", result.Projects)
	}
}

// TestScan_ErrorsWhenTheRootIsUnreadable checks the other half of the walk
// error handler: an unreadable root is not skipped, because that would report
// an empty scan as a success.
func TestScan_ErrorsWhenTheRootIsUnreadable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n")
	if err := os.Chmod(root, 0); err != nil {
		t.Fatalf("chmod %s: %v", root, err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if _, err := os.ReadDir(root); err == nil {
		t.Skip("permission bits are not enforced (running as root?); cannot exercise an unreadable root")
	}

	if _, err := Scan(root); err == nil {
		t.Fatal("Scan() of an unreadable root must return an error")
	}
}

func TestScan_ErrorsWhenThePathIsNotADirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir.txt")
	writeFile(t, path, "hello")

	_, err := Scan(path)
	if err == nil {
		t.Fatal("Scan() of a file must return an error")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("Scan() error = %q, want it to say the path is not a directory", err)
	}
}

// TestScan_ErrorsWhenTheWorkingDirectoryCannotBeResolved covers filepath.Abs
// failing for a relative root: rather than inventing a root under an
// unresolvable working directory, Scan must surface the error.
func TestScan_ErrorsWhenTheWorkingDirectoryCannotBeResolved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the working directory cannot be made unresolvable this way on Windows")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.Getwd(); err == nil {
		t.Skip("the working directory is still resolvable; cannot exercise filepath.Abs's error")
	}

	if _, err := Scan("relative-root"); err == nil {
		t.Fatal("Scan() must fail when the working directory cannot be resolved")
	}
}

// TestScan_SkipsUnreadableManifests checks that a manifest the process cannot
// read is skipped silently: the file exists, so the walk reports it, but it
// contributes no project.
func TestScan_SkipsUnreadableManifests(t *testing.T) {
	root := t.TempDir()
	goMod := filepath.Join(root, "go.mod")
	writeFile(t, goMod, "module example.com/root\n")
	pkgJSON := filepath.Join(root, "package.json")
	writeFile(t, pkgJSON, `{"name":"app","scripts":{"test":"jest"}}`)

	for _, path := range []string{goMod, pkgJSON} {
		if err := os.Chmod(path, 0); err != nil {
			t.Fatalf("chmod %s: %v", path, err)
		}
		p := path
		t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	}
	if _, err := os.ReadFile(goMod); err == nil {
		t.Skip("permission bits are not enforced on files (running as root?); cannot exercise unreadable manifests")
	}

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v; an unreadable manifest must be skipped, not fail the scan", err)
	}
	if len(result.Projects) != 0 {
		t.Errorf("Projects = %+v, want none: unreadable manifests contribute no project", result.Projects)
	}
}

// TestScan_SkipsManifestsWithoutUsableContent covers both parse failures: a
// go.mod with no module directive and a package.json that is not valid JSON.
func TestScan_SkipsManifestsWithoutUsableContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "go 1.22\n")
	writeFile(t, filepath.Join(root, "pkg", "package.json"), "{not json")

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v; unparseable manifests must be skipped, not fail the scan", err)
	}
	if len(result.Projects) != 0 {
		t.Errorf("Projects = %+v, want none: a module-less go.mod and malformed JSON are both skipped", result.Projects)
	}
}

func TestReadModulePath(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr string
	}{
		{"plain", "module example.com/plain\n\ngo 1.22\n", "example.com/plain", ""},
		{"quoted", "module \"example.com/quoted\"\n", "example.com/quoted", ""},
		{"surrounding whitespace and tabs", "\t module\texample.com/spaced\t\n", "example.com/spaced", ""},
		{"only a comment", "// module example.com/commented\n", "", "no module directive"},
		{"no directive", "go 1.22\n", "", "no module directive"},
		{"empty directive", "module\n", "", "no module directive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".mod")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := readModulePath(path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("readModulePath(%q) error = %v, want it to contain %q", tc.body, err, tc.wantErr)
				}
				if got != "" {
					t.Errorf("readModulePath(%q) = %q on error, want \"\"", tc.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readModulePath(%q) error = %v", tc.body, err)
			}
			if got != tc.want {
				t.Errorf("readModulePath(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestReadModulePath_ReportsReadErrors(t *testing.T) {
	if _, err := readModulePath(filepath.Join(t.TempDir(), "absent.mod")); err == nil {
		t.Fatal("readModulePath() of a missing file must return an error")
	}
}

func TestReadPackageJSON(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	writeFile(t, valid, `{"name":"app","scripts":{"test":"vitest run"},"dependencies":{"vitest":"^2"}}`)

	pkg, err := readPackageJSON(valid)
	if err != nil {
		t.Fatalf("readPackageJSON(valid) error = %v", err)
	}
	if pkg.Name != "app" {
		t.Errorf("Name = %q, want app", pkg.Name)
	}
	if pkg.Scripts["test"] != "vitest run" {
		t.Errorf("Scripts[test] = %q, want \"vitest run\"", pkg.Scripts["test"])
	}
	if pkg.Dependencies["vitest"] != "^2" {
		t.Errorf("Dependencies[vitest] = %q, want ^2", pkg.Dependencies["vitest"])
	}

	malformed := filepath.Join(dir, "malformed.json")
	writeFile(t, malformed, "{not json")
	_, err = readPackageJSON(malformed)
	if err == nil {
		t.Fatal("readPackageJSON() of malformed JSON must return an error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("readPackageJSON() error = %q, want it to name the parse failure", err)
	}

	if _, err := readPackageJSON(filepath.Join(dir, "absent.json")); err == nil {
		t.Fatal("readPackageJSON() of a missing file must return an error")
	}
}

func TestDetectRunner(t *testing.T) {
	tests := []struct {
		name string
		pkg  packageJSON
		want string
	}{
		{"vitest in devDependencies",
			packageJSON{DevDependencies: map[string]string{"vitest": "^2"}}, "vitest"},
		{"jest in devDependencies",
			packageJSON{DevDependencies: map[string]string{"jest": "^29"}}, "jest"},
		{"vitest in dependencies",
			packageJSON{Dependencies: map[string]string{"vitest": "^2"}}, "vitest"},
		{"jest in dependencies",
			packageJSON{Dependencies: map[string]string{"jest": "^29"}}, "jest"},
		{"vitest named by the test script",
			packageJSON{Scripts: map[string]string{"test": "vitest run"}}, "vitest"},
		{"jest named by the test script",
			packageJSON{Scripts: map[string]string{"test": "jest --ci"}}, "jest"},
		{"no runner anywhere",
			packageJSON{Scripts: map[string]string{"test": "node test.js"}}, ""},
		{"declared jest beats a script naming vitest",
			packageJSON{
				Scripts:         map[string]string{"test": "vitest run"},
				DevDependencies: map[string]string{"jest": "^29"},
			}, "jest"},
		{"declared vitest beats a script naming jest",
			packageJSON{
				Scripts:      map[string]string{"test": "jest --ci"},
				Dependencies: map[string]string{"vitest": "^2"},
			}, "vitest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectRunner(tc.pkg); got != tc.want {
				t.Errorf("detectRunner(%+v) = %q, want %q", tc.pkg, got, tc.want)
			}
		})
	}
}

// TestScan_SortsProjectsInOneDirectoryByKind covers the second sort key: a
// directory holding both a go.mod and a package.json yields two projects with
// the same Dir, and their order must be the deterministic Go-before-Node one.
func TestScan_SortsProjectsInOneDirectoryByKind(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n")
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"app","scripts":{"test":"vitest run"}}`)

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(result.Projects) != 2 {
		t.Fatalf("Projects = %+v, want a Go module and a Node package", result.Projects)
	}
	if result.Projects[0].Dir != result.Projects[1].Dir {
		t.Fatalf("projects are not in the same directory: %+v", result.Projects)
	}
	if result.Projects[0].Kind != KindGo || result.Projects[1].Kind != KindNode {
		t.Errorf("project kinds = %q, %q, want go then node: same-directory projects sort by Kind",
			result.Projects[0].Kind, result.Projects[1].Kind)
	}
}

func TestRelSlash(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "services", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, root, dir, want string
	}{
		{"directory is the root", root, root, "."},
		{"nested directory", root, nested, "services/api"},
		{"relative root and absolute directory", "relative/root", filepath.Join(root, "x"), "."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relSlash(tc.root, tc.dir); got != tc.want {
				t.Errorf("relSlash(%q, %q) = %q, want %q", tc.root, tc.dir, got, tc.want)
			}
		})
	}
}

// TestScan_KeepsHiddenRootAndFindsNestedManifests covers the p != abs guard:
// only hidden *children* are skipped, so a scan root whose own name starts
// with a dot is still walked. It also exercises slash-separated Rel values for
// nested manifests and the JSON-config and extension language flags.
func TestScan_KeepsHiddenRootAndFindsNestedManifests(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".hidden-root")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/top\n")
	writeFile(t, filepath.Join(root, "services", "api", "go.mod"), "module example.com/api\n")
	writeFile(t, filepath.Join(root, "web", "app", "package.json"),
		`{"name":"web-app","scripts":{"test":"vitest run"},"devDependencies":{"vitest":"^2"}}`)
	writeFile(t, filepath.Join(root, "web", "app", "jsconfig.json"), "{}")
	writeFile(t, filepath.Join(root, "web", "app", "main.mjs"), "export const x = 1\n")
	writeFile(t, filepath.Join(root, "web", "app", "view.tsx"), "export const y = 1\n")

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	modules := result.GoModules()
	if len(modules) != 2 {
		t.Fatalf("GoModules() = %+v, want the hidden root and the nested module", modules)
	}
	rels := map[string]string{}
	for _, m := range modules {
		rels[m.Rel] = m.ModulePath
	}
	if rels["."] != "example.com/top" {
		t.Errorf("root module = %q, want example.com/top (a hidden root directory must still be scanned)", rels["."])
	}
	if rels["services/api"] != "example.com/api" {
		t.Errorf("nested module = %q, want example.com/api with a slash-separated Rel", rels["services/api"])
	}

	node := result.NodePackages()
	if len(node) != 1 || node[0].Rel != "web/app" {
		t.Fatalf("NodePackages() = %+v, want one package with Rel web/app", node)
	}
	if node[0].Runner != "vitest" {
		t.Errorf("Runner = %q, want vitest", node[0].Runner)
	}
	if !result.HasTypeScript || !result.HasJavaScript {
		t.Errorf("HasTypeScript=%v HasJavaScript=%v, want both true (jsconfig.json, .tsx and .mjs files exist)",
			result.HasTypeScript, result.HasJavaScript)
	}
}
