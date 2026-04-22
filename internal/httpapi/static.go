package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// webAssets holds the compiled SvelteKit single-page app. `build/` must exist at
// compile time. It is produced by `npm run build` in ../web.
//
//go:embed all:webassets
var webAssetsFS embed.FS

// StaticHandler returns an http.Handler that serves the embedded SPA:
//   - /api/* is NOT served here — that's routed above.
//   - existing files are served as-is.
//   - anything else (unknown path) falls back to index.html so the client router
//     can interpret it — the SvelteKit adapter-static pattern.
func StaticHandler() (http.Handler, error) {
	sub, err := fs.Sub(webAssetsFS, "webassets")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(sub, p); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for unknown paths so client router can handle it.
		index, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	}), nil
}
