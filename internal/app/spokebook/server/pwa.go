package server

import (
	"encoding/json"
	"net/http"

	"github.com/spokebook/spokebook/web"
)

// serviceWorker serves web/sw.js with the app version in front, so a new
// release replaces the cached offline page. It sits at the root so its
// scope covers every page.
func (s *Server) serviceWorker(w http.ResponseWriter, r *http.Request) {
	version, err := json.Marshal(s.version)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte("const VERSION = " + string(version) + ";\n"))
	_, _ = w.Write(web.ServiceWorker)
}

// offline is the page the service worker caches and shows when the server
// can't be reached.
func (s *Server) offline(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "offline.html", map[string]any{})
}
