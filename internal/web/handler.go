package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// NewSPAHandler returns an HTTP handler for the SPA.
// All requests not starting with /api/v1 are served from the embedded FS.
// If a file is not found, index.html is served (for SPA routing).
func NewSPAHandler() http.Handler {
	sub, err := fs.Sub(UIFS, "ui")
	if err != nil {
		panic(err)
	}

	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Root path — serve index.html directly
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(indexBytes)
			return
		}

		// Try to open the requested file
		p := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := sub.Open(p); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: for unknown paths, serve index.html
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
}
