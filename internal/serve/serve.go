// Package serve starts cover100's local static server and opens the report in
// the user's browser.
//
// The server is bound to loopback only: the report describes a private
// codebase, and nothing about viewing it requires exposing it to the network.
package serve

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sneat-dev/cover100-cli/internal/ui"
)

// portAttempts is how many consecutive ports are tried before giving up, so a
// stale server on the default port does not require the user to find a free one.
const portAttempts = 20

// Options configures the server.
type Options struct {
	// Assets is a filesystem rooted at the public directory.
	Assets fs.FS
	// DataPath is the absolute path of the report JSON to serve.
	DataPath string
	// DataRoute is the URL path the report is served at, e.g. "/coverage.json".
	DataRoute string
	// Host defaults to 127.0.0.1.
	Host string
	// Port is the preferred port; the server walks forward from it when busy.
	Port int
	// Printer receives the bound URL and diagnostics.
	Printer *ui.Printer
}

// Server is a running static server.
type Server struct {
	httpSrv *http.Server
	listen  net.Listener
	url     string
	port    int
}

// Start binds the server and returns it ready to Serve. The returned URL is
// the address the browser should open, including the data query parameter.
func Start(opts Options) (*Server, error) {
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	dataRoute := opts.DataRoute
	if dataRoute == "" {
		dataRoute = "/coverage.json"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(dataRoute, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.ServeFile(w, r, opts.DataPath)
	})
	fileServer := http.FileServerFS(opts.Assets)
	mux.Handle("/", fileServer)

	listen, port, err := listenLoopback(host, opts.Port)
	if err != nil {
		return nil, err
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if opts.Printer != nil && port != opts.Port {
		opts.Printer.Detail("port %d was busy; using %d", opts.Port, port)
	}
	return &Server{
		httpSrv: srv,
		listen:  listen,
		port:    port,
		url:     fmt.Sprintf("http://%s%s", listen.Addr().String(), dataRoute),
	}, nil
}

// listenLoopback binds the first free port at or after preferred.
//
// The returned port is read back from the listener rather than assumed: with
// preferred 0 the kernel picks the port, and reporting 0 would send the browser
// nowhere.
func listenLoopback(host string, preferred int) (net.Listener, int, error) {
	var lastErr error
	for offset := 0; offset < portAttempts; offset++ {
		wanted := preferred + offset
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, wanted))
		if err != nil {
			lastErr = err
			continue
		}
		port := wanted
		if tcp, ok := ln.Addr().(*net.TCPAddr); ok && tcp.Port != 0 {
			port = tcp.Port
		}
		return ln, port, nil
	}
	return nil, 0, fmt.Errorf("no free port in %d..%d: %w", preferred, preferred+portAttempts-1, lastErr)
}

// URL returns the address of the served report.
func (s *Server) URL() string { return s.url }

// PageURL returns the viewer page address, with the report URL, metric and
// mode passed as query parameters so the CLI's defaults and the page's initial
// state cannot disagree.
func (s *Server) PageURL(metric, mode string) string {
	base := fmt.Sprintf("http://%s/", s.listen.Addr().String())
	params := []string{"data=" + s.dataPath()}
	if metric != "" {
		params = append(params, "metric="+metric)
	}
	if mode != "" {
		params = append(params, "mode="+mode)
	}
	return base + "?" + strings.Join(params, "&")
}

// Port returns the port the server actually bound.
func (s *Server) Port() int { return s.port }

func (s *Server) dataPath() string {
	// s.url is http://host:port<dataRoute>; the viewer wants just the path.
	if i := strings.Index(s.url, "://"); i >= 0 {
		if j := strings.Index(s.url[i+3:], "/"); j >= 0 {
			return s.url[i+3+j:]
		}
	}
	return "/coverage.json"
}

// Serve blocks until the server is closed.
func (s *Server) Serve() error {
	err := s.httpSrv.Serve(s.listen)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close shuts the server down, waiting for in-flight requests briefly.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

// OpenBrowser opens url in the platform's default browser. It returns an error
// only when no launcher could be started; a launcher that starts but fails is
// the user's environment, not a cover100 failure, so the URL is always printed.
func OpenBrowser(url string) error {
	cmd := browserCommand(url)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %s: %w", url, err)
	}
	// Reap the launcher so it does not linger as a zombie; its exit status is
	// irrelevant to us.
	go func() { _ = cmd.Wait() }()
	return nil
}
