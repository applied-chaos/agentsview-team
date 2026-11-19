//go:build testclock

package instance

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
)

var sabotaged sync.Map

func beforeStart(userID string) error {
	if _, ok := sabotaged.Load(userID); ok {
		return errors.New("sabotaged by test")
	}
	return nil
}

// testRoutes exposes POST /_test/instances/{id}/kill and /sabotage {"on":bool}.
// Compiled in only with -tags testclock, like the test clock.
func (m *Manager) testRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /_test/instances/{id}/kill", func(w http.ResponseWriter, r *http.Request) {
		if h := m.handle(r.PathValue("id")); h != nil {
			h.mu.Lock()
			if h.cmd != nil && h.cmd.Process != nil {
				_ = h.cmd.Process.Kill()
			}
			h.mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_test/instances/{id}/sabotage", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if body.On {
			sabotaged.Store(r.PathValue("id"), true)
		} else {
			sabotaged.Delete(r.PathValue("id"))
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
