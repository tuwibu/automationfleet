// Package testpage spins up an HTTP server serving an instrumented form
// page used by the automationfleet examples.
package testpage

import (
	_ "embed"
	"fmt"
	"net"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

// Server wraps a running net/http server bound to a random localhost port.
type Server struct {
	URL    string
	server *http.Server
	ln     net.Listener
}

// Start opens a TCP listener on 127.0.0.1:<random>, returns a Server with the
// fully-qualified URL.
func Start() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexHTML)
	})
	s := &http.Server{Handler: mux}
	go func() { _ = s.Serve(ln) }()
	return &Server{
		URL:    fmt.Sprintf("http://%s", ln.Addr().String()),
		server: s,
		ln:     ln,
	}, nil
}

// PageURL returns the test-page URL with a window ID query param so each
// browser instance can label its events independently.
func (s *Server) PageURL(id string) string {
	return s.URL + "/?id=" + id
}

// Close shuts the server down.
func (s *Server) Close() error { return s.server.Close() }
