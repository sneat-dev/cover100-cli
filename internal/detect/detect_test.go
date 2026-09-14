package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file, creating parent directories as needed.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_FindsModulesAndPackages(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "web", "package.json"),
		`{"name":"web","scripts":{"test":"vitest run"},"devDependencies":{"vitest":"^2"}}`)
	writeFile(t, filepath.Join(root, "web", "tsconfig.json"), "{}")
	writeFile(t, filepath.Join(root, "web", "src", "a.ts"), "export const a = 1\n")
	writeFile(t, filepath.Join(root, "legacy", "package.json"),
		`{"name":"legacy","scripts":{"build":"tsc"}}`)
	writeFile(t, filepath.Join(root, "legacy", "index.js"), "module.exports = {}\n")

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if got := len(result.GoModules()); got != 1 {
		t.Errorf("GoModules() = %d, want 1", got)
	}
	gomod := result.GoModules()[0]
	if gomod.ModulePath != "example.com/root" {
		t.Errorf("ModulePath = %q", gomod.ModulePath)
	}
	if gomod.Rel != "." {
		t.Errorf("root module Rel = %q, want .", gomod.Rel)
	}

	// Only the package with a test script is runnable.
	node := result.NodePackages()
	if len(node) != 1 {
		t.Fatalf("NodePackages() = %d, want 1 (legacy has no test script)", len(node))
	}
	if node[0].Runner != "vitest" {
		t.Errorf("Runner = %q, want vitest", node[0].Runner)
	}
	if node[0].Rel != "web" {
		t.Errorf("Rel = %q, want web", node[0].Rel)
	}
	if !result.HasTypeScript {
		t.Error("HasTypeScript = false, want true (tsconfig.json and a .ts file exist)")
	}
	if !result.HasJavaScript {
		t.Error("HasJavaScript = false, want true (legacy/index.js exists)")
	}
}

func TestScan_SkipsDependencyAndHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n")
	writeFile(t, filepath.Join(root, "node_modules", "dep", "package.json"),
		`{"name":"dep","scripts":{"test":"jest"}}`)
	writeFile(t, filepath.Join(root, "vendor", "pkg", "go.mod"), "module example.com/vendored\n")
	writeFile(t, filepath.Join(root, ".hidden", "package.json"),
		`{"name":"hidden","scripts":{"test":"jest"}}`)

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got := len(result.Projects); got != 1 {
		t.Fatalf("Projects = %d, want only the root module: %+v", got, result.Projects)
	}
}

func TestScan_DetectsRunnerFromScriptWhenDependencyIsAbsent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"),
		`{"name":"app","scripts":{"test":"jest --ci"}}`)

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got := result.NodePackages(); len(got) != 1 || got[0].Runner != "jest" {
		t.Fatalf("NodePackages() = %+v, want one jest project", got)
	}
}

func TestScan_ErrorsOnMissingPath(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("Scan() of a missing path must return an error")
	}
}

func TestScan_ReportsTestScriptLessPackagesAsNotRunnable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"app"}`)

	result, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got := len(result.NodePackages()); got != 0 {
		t.Errorf("NodePackages() = %d, want 0", got)
	}
	if got := len(result.Projects); got != 1 {
		t.Errorf("Projects = %d, want 1 (detected but not runnable)", got)
	}
}
