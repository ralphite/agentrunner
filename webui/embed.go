package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// distFS holds the Vite-built React frontend. `frontend/dist` must contain at
// least an index.html at compile time (a committed placeholder suffices until
// the real build runs — see frontend/README).
//
//go:embed all:frontend/dist
var distFS embed.FS

// staticHandler serves the embedded SPA: real asset paths resolve to files,
// everything else falls back to index.html (client-side routing / hash nav).
func staticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			// Unknown path: hand back index.html so the SPA can route it.
			http.ServeFileFS(w, r, sub, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
