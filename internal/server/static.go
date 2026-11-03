package server

import (
	"io/fs"
	"net/http"
	"path"
	"strconv"
)

var contentTypes = map[string]string{
	".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml",
	".png": "image/png", ".woff2": "font/woff2", ".woff": "font/woff", ".json": "application/json",
}

// static serves the built page. Any path that is not a file of the page (share links /s/<token>, ?b=<board>) gets
// index.html, which picks its board from the URL.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Dist == nil {
		http.Error(w, "page not built: run make build", http.StatusInternalServerError)

		return
	}

	name := cleanStaticPath(r.URL.Path)

	if st, err := fs.Stat(s.cfg.Dist, name); err != nil || st.IsDir() || name == "" {
		name = "index.html"
	}

	data, err := fs.ReadFile(s.cfg.Dist, name)
	if err != nil {
		http.Error(w, "page not built: run make build", http.StatusInternalServerError)

		return
	}

	ct := contentTypes[path.Ext(name)]
	if ct == "" {
		ct = "application/octet-stream"
	}

	cache := "max-age=31536000, immutable"
	if name == "index.html" {
		cache = "no-cache"
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)

	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}
