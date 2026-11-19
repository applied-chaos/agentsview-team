//go:build testclock

package stats

import "net/http"

// TestRoutes exposes POST /_test/stats/refresh, a synchronous full refresh.
func (s *Service) TestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /_test/stats/refresh", func(w http.ResponseWriter, r *http.Request) {
		s.Refresh(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
}
