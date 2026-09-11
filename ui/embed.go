// Package ui serves the embedded client and operator frontends.
package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var output embed.FS

func page(name string) http.Handler {
	body, err := output.ReadFile("dist/" + name + "/index.html")
	if err != nil {
		panic(fmt.Errorf("read %s UI (run make ui): %w", name, err))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if r.Method != http.MethodHead {
			w.Write(body)
		}
	})
}

func ClientPage() http.Handler   { return page("client") }
func OperatorPage() http.Handler { return page("operator") }

// Assets serves common build assets relative to /ui/ or /_internal/ui/.
// The caller strips its prefix and applies the interface's access controls.
func Assets() http.Handler {
	files, err := fs.Sub(output, "dist")
	if err != nil {
		panic(err)
	}
	server := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if !fs.ValidPath(name) || strings.HasSuffix(name, "/") ||
			(!strings.HasPrefix(name, "assets/") && !strings.HasPrefix(name, "fonts/")) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		server.ServeHTTP(w, r)
	})
}
