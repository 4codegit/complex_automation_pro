// Package web embeds the built dashboard (index.html + assets) so a single
// AutoPro server process serves both the API and the UI on the same origin.
//
// Regenerate the embedded bundle from the frontend after any change:
//
//	cd frontend && npm run build && cp -r dist/* ../backend/internal/web/web/
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:web
var content embed.FS

// Handler serves the SPA: the embedded directory root maps to index.html,
// hashed assets are served directly, and any unknown non-API path falls back
// to index.html so client-side routing works.
func Handler() http.Handler {
	sub, err := fs.Sub(content, "web")
	if err != nil {
		panic("web: no embedded web dir: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never intercept API, WebSocket or explicitly-banned plumbing paths.
		if isAPI(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		path := r.URL.Path
		if !strings.HasPrefix(path, "/assets/") {
			// SPA route: let FileServer's directory rendering emit index.html.
			path = "/"
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = path
		fileServer.ServeHTTP(w, r2)
	})
}

func isAPI(path string) bool {
	return path == "/ws" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/api")
}
