package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// NewSPAHandler возвращает HTTP-обработчик для SPA.
// Все запросы, не начинающиеся с /api/v1, обслуживаются из embedded FS.
// Если файл не найден — отдаётся index.html (для SPA-роутинга).
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
		// Root path — сразу отдаём index.html напрямую (без FileServer, чтобы избежать 301)
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(indexBytes)
			return
		}

		// Пробуем открыть запрошенный файл
		p := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := sub.Open(p); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: для неизвестных путей отдаём index.html
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
}
