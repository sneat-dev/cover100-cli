package serve

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><body>viewer</body></html>")},
		"app.js":     &fstest.MapFile{Data: []byte("// app")},
		"styles.css": &fstest.MapFile{Data: []byte("/* css */")},
	}
}

func startTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Port 0 lets the OS choose a free port, so the test never collides with a
	// developer's running server.
	srv, err := Start(Options{Assets: testAssets(), DataPath: dataPath, Port: 0})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	go func() { _ = srv.Serve() }()
	return srv, dataPath
}

func TestServer_ServesReportAndAssets(t *testing.T) {
	srv, _ := startTestServer(t)

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("GET %s: %v", srv.URL(), err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store so a re-run is never stale", got)
	}
}

func TestServer_ServesViewerAtRoot(t *testing.T) {
	srv, _ := startTestServer(t)

	base := srv.URL()[:strings.LastIndex(srv.URL(), "/")]
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "viewer") {
		t.Errorf("root did not serve index.html: %q", body)
	}
}

func TestServer_BindsLoopbackOnly(t *testing.T) {
	srv, _ := startTestServer(t)
	addr := srv.listen.Addr().String()
	host, _, ok := strings.Cut(addr, ":")
	if !ok {
		t.Fatalf("unexpected listener address %q", addr)
	}
	if host != "127.0.0.1" {
		t.Errorf("listener host = %q, want 127.0.0.1: the report must not be exposed on the network", host)
	}
}

func TestPageURL_CarriesDataMetricAndMode(t *testing.T) {
	srv, _ := startTestServer(t)
	page := srv.PageURL("functions", "count")

	for _, want := range []string{"data=/coverage.json", "metric=functions", "mode=count"} {
		if !strings.Contains(page, want) {
			t.Errorf("PageURL() = %q, missing %q", page, want)
		}
	}
}

func TestStart_FallsForwardWhenPortIsBusy(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Start(Options{Assets: testAssets(), DataPath: dataPath, Port: 0})
	if err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer func() { _ = first.Close() }()

	second, err := Start(Options{Assets: testAssets(), DataPath: dataPath, Port: first.Port()})
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	defer func() { _ = second.Close() }()

	if second.Port() == first.Port() {
		t.Fatalf("second server reused the busy port %d", first.Port())
	}
}
