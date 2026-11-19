// Package server assembles the Platform's HTTP surface: identity routes, the
// JSON API and the SPA.
package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/applied-chaos/agentsview-team/internal/admin"
	"github.com/applied-chaos/agentsview-team/internal/auth"
	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/hosts"
	"github.com/applied-chaos/agentsview-team/internal/instance"
	"github.com/applied-chaos/agentsview-team/internal/replay"
	"github.com/applied-chaos/agentsview-team/internal/stats"
)

func New(a *auth.Service, adm *admin.Service, clk *clock.Clock, inst *instance.Manager, hs *hosts.Service, rp *replay.Service, ts *stats.Service, webDir string) http.Handler {
	mux := http.NewServeMux()
	a.Routes(mux)
	adm.Routes(mux)
	registerTestClock(mux, clk)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	spaHandler := spa(webDir)
	hs.Routes(mux)
	inst.Routes(mux, a, hs, spaHandler, webDir)
	rp.TestRoutes(mux)
	ts.Routes(mux)
	ts.TestRoutes(mux)
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/", spaHandler)
	return protect(errorPages(webDir, mux))
}

func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		p := path.Clean("/" + r.URL.Path)
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err == nil && !st.IsDir() {
			if strings.HasPrefix(p, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
