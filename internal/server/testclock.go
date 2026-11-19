//go:build testclock

package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/clock"
)

// registerTestClock exposes POST /_test/clock {"advance":"16m"}. Compiled in
// only with -tags testclock; main refuses to start such a build in production.
func registerTestClock(mux *http.ServeMux, clk *clock.Clock) {
	mux.HandleFunc("POST /_test/clock", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Advance string `json:"advance"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		d, err := time.ParseDuration(body.Advance)
		if err != nil || d < 0 {
			http.Error(w, "advance must be a non-negative duration", http.StatusBadRequest)
			return
		}
		clk.Advance(d)
		w.WriteHeader(http.StatusNoContent)
	})
}
