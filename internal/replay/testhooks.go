//go:build testclock

package replay

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
)

type testHooks struct{ crash atomic.Bool }

var errCrash = errors.New("replay: test crash after commit")

func (h *testHooks) crashAfterCommit() error {
	if h.crash.CompareAndSwap(true, false) {
		return errCrash
	}
	return nil
}

func (*testHooks) isCrash(err error) bool { return errors.Is(err, errCrash) }

// TestRoutes exposes POST /_test/replay {"paused":bool,"crashAfterCommit":bool}. Pausing stops
// the worker; un-pausing starts a fresh one, which is a restart. Compiled in only with -tags testclock.
func (s *Service) TestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /_test/replay", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Paused           *bool `json:"paused"`
			CrashAfterCommit *bool `json:"crashAfterCommit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if body.CrashAfterCommit != nil {
			s.hooks.crash.Store(*body.CrashAfterCommit)
		}
		if body.Paused != nil {
			if *body.Paused {
				s.Stop()
			} else {
				s.Start(s.root)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
