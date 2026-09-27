// Package web serves the job search page. It's plain HTML, CSS and
// JavaScript embedded into the Go binary, so there's no separate frontend
// build and the whole app still ships as one file.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// Handler serves the search page at / and its assets under /static/.
func Handler() http.Handler {
	static, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // the embedded directory is fixed at compile time
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, "index.html")
	})
	return securityHeaders(mux)
}

// securityHeaders tells the browser to only run scripts and styles from our
// own files. Job titles and descriptions come from third-party sites, so if
// one ever contained a <script> tag and a bug let it into the page, the
// browser would still refuse to run it.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
