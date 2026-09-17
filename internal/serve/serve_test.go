package serve

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sneat-dev/cover100-cli/internal/ui"
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
	defer func() { _ = resp.Body.Close() }()
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
	defer func() { _ = resp.Body.Close() }()
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

func TestStart_CustomDataRoute(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{"custom":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, err := Start(Options{Assets: testAssets(), DataPath: dataPath, DataRoute: "/report.json", Port: 0})
	if err != nil {
		t.Fatalf("Start() with a custom DataRoute error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	go func() { _ = srv.Serve() }()

	if want := "/report.json"; !strings.HasSuffix(srv.URL(), want) {
		t.Errorf("URL() = %q, want it to end with the custom data route %q", srv.URL(), want)
	}
	if page := srv.PageURL("", ""); strings.Contains(page, "metric=") || strings.Contains(page, "mode=") {
		t.Errorf("PageURL(\"\", \"\") = %q, want no empty metric or mode parameter", page)
	}

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("GET %s: %v", srv.URL(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the custom data route", resp.StatusCode)
	}
	if string(body) != `{"custom":true}` {
		t.Errorf("custom data route body = %q, want the report", body)
	}
}

func TestStart_EmptyHostDefaultsToLoopback(t *testing.T) {
	srv, _ := startTestServer(t)
	if !strings.HasPrefix(srv.URL(), "http://127.0.0.1:") {
		t.Errorf("URL() = %q, want an empty Host to default to 127.0.0.1", srv.URL())
	}
}

func TestStart_ReportsBusyPortToPrinter(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a busy port: %v", err)
	}
	t.Cleanup(func() { _ = busy.Close() })
	port := busy.Addr().(*net.TCPAddr).Port
	if port >= 65535 {
		t.Skipf("ephemeral port %d has no successor to fall forward to", port)
	}

	var out, errOut bytes.Buffer
	printer := ui.New(&out, &errOut, false)
	srv, err := Start(Options{
		Assets:   testAssets(),
		DataPath: dataPath,
		Host:     "127.0.0.1",
		Port:     port,
		Printer:  printer,
	})
	if err != nil {
		t.Fatalf("Start() on a busy port error = %v", err)
	}
	defer func() { _ = srv.Close() }()

	if srv.Port() == port {
		t.Fatalf("Start() bound the occupied port %d instead of falling forward", port)
	}
	want := fmt.Sprintf("port %d was busy; using %d", port, srv.Port())
	if !strings.Contains(out.String(), want) {
		t.Errorf("printer output = %q, want the busy-port detail %q", out.String(), want)
	}
}

// occupyLoopbackRange binds n consecutive loopback ports so tests can exercise
// the "every candidate is taken" path without ever touching a developer's
// machine beyond its own ephemeral range. The listeners are returned for the
// caller to close.
func occupyLoopbackRange(t *testing.T, n int) (int, []net.Listener) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		first, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserving a base port: %v", err)
		}
		base := first.Addr().(*net.TCPAddr).Port
		listeners := []net.Listener{first}
		ok := true
		for i := 1; i < n; i++ {
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", base+i))
			if err != nil {
				ok = false
				break
			}
			listeners = append(listeners, ln)
		}
		if ok {
			return base, listeners
		}
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}
	t.Skipf("could not reserve %d contiguous loopback ports on this machine", n)
	return 0, nil
}

func closeListeners(t *testing.T, listeners []net.Listener) {
	t.Helper()
	for _, ln := range listeners {
		t.Cleanup(func() { _ = ln.Close() })
	}
}

func TestListenLoopback_ExhaustedRange(t *testing.T) {
	base, listeners := occupyLoopbackRange(t, portAttempts)
	closeListeners(t, listeners)

	ln, port, err := listenLoopback("127.0.0.1", base)
	if err == nil {
		if ln != nil {
			_ = ln.Close()
		}
		t.Fatalf("listenLoopback() bound port %d from a fully occupied %d..%d range", port, base, base+portAttempts-1)
	}
	if ln != nil {
		t.Errorf("listenLoopback() returned a listener %v alongside error %v", ln.Addr(), err)
	}
	if port != 0 {
		t.Errorf("port = %d, want 0 when no port could be bound", port)
	}
	wantRange := fmt.Sprintf("no free port in %d..%d", base, base+portAttempts-1)
	if !strings.Contains(err.Error(), wantRange) {
		t.Errorf("error = %v, want it to name the exhausted range %q", err, wantRange)
	}
}

func TestStart_FailsWhenEveryPortIsTaken(t *testing.T) {
	base, listeners := occupyLoopbackRange(t, portAttempts)
	closeListeners(t, listeners)

	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, err := Start(Options{Assets: testAssets(), DataPath: dataPath, Host: "127.0.0.1", Port: base})
	if err == nil {
		_ = srv.Close()
		t.Fatalf("Start() succeeded on a fully occupied %d..%d range", base, base+portAttempts-1)
	}
	if srv != nil {
		t.Errorf("Start() returned server %+v alongside error %v", srv, err)
	}
	if !strings.Contains(err.Error(), "no free port in") {
		t.Errorf("Start() error = %v, want the listenLoopback exhaustion error", err)
	}
}

func TestDataPath_ReturnsRoutePath(t *testing.T) {
	srv := &Server{url: "http://127.0.0.1:8080/coverage.json"}
	if got, want := srv.dataPath(), "/coverage.json"; got != want {
		t.Errorf("dataPath() = %q, want %q", got, want)
	}
}

func TestDataPath_FallsBackToDefaultRoute(t *testing.T) {
	// A URL with no scheme, and one with a scheme but no path, both have to
	// fall back rather than slice into the authority.
	for _, rawURL := range []string{"coverage.json", "http://127.0.0.1:8080", "http://host"} {
		t.Run(rawURL, func(t *testing.T) {
			srv := &Server{url: rawURL}
			if got, want := srv.dataPath(), "/coverage.json"; got != want {
				t.Errorf("dataPath() for url %q = %q, want the fallback %q", rawURL, got, want)
			}
		})
	}
}

func TestServe_ReturnsNilAfterClose(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := Start(Options{Assets: testAssets(), DataPath: dataPath, Port: 0})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	if err := srv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve() = %v, want nil: a deliberate Close must not be reported as a server failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return within 5s of Close()")
	}
}

func TestServe_ReportsListenerFailure(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "coverage.json")
	if err := os.WriteFile(dataPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := Start(Options{Assets: testAssets(), DataPath: dataPath, Port: 0})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	// Closing the raw listener is not a deliberate Shutdown, so the accept
	// failure must surface instead of being mistaken for ErrServerClosed.
	if err := srv.listen.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Serve() = nil after its listener broke, want the accept error so a real failure is never swallowed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return within 5s of its listener closing")
	}
	_ = srv.Close()
}

// launcherName is the command OpenBrowser shells out to on this platform.
func launcherName() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "windows":
		return "rundll32"
	default:
		return "xdg-open"
	}
}

func TestOpenBrowser_StartsTheLauncherWithTheURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub launcher is a POSIX shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened.txt")
	stub := filepath.Join(dir, launcherName())
	// The stub records the URL it was handed and exits; it must never resolve
	// to a real browser because PATH is replaced with the stub directory.
	script := "#!/bin/sh\necho \"$1\" > \"$COVER100_OPEN_MARKER\"\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub launcher: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("COVER100_OPEN_MARKER", marker)

	const url = "http://127.0.0.1:9999/?data=/coverage.json"
	if err := OpenBrowser(url); err != nil {
		t.Fatalf("OpenBrowser(%q) error = %v", url, err)
	}

	// OpenBrowser reaps the launcher in the background, so wait for the stub's
	// marker rather than assuming it has already been written.
	deadline := time.Now().Add(5 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		if got, _ = os.ReadFile(marker); len(got) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.TrimSpace(string(got)) != url {
		t.Errorf("stub launcher received %q, want the URL %q", strings.TrimSpace(string(got)), url)
	}
}

func TestOpenBrowser_NoLauncherNamesTheURL(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	const url = "http://127.0.0.1:8123/?data=/coverage.json"
	err := OpenBrowser(url)
	if err == nil {
		t.Fatal("OpenBrowser() with no launcher on PATH must report the failure")
	}
	if !strings.Contains(err.Error(), url) {
		t.Errorf("error = %v, want it to include the URL %q so the caller can print it", err, url)
	}
}
