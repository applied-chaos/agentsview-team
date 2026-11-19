package admin

import (
	"net/http"
	"strings"

	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

func (s *Service) target(r *http.Request, by store.Actor) (store.UserRow, error) {
	u, err := s.store.UserRowByID(r.Context(), r.PathValue("id"))
	if err == nil && by.ID != nil && *by.ID == u.ID {
		err = errSelf
	}
	return u, err
}

// disable runs its side effects even when the User is already Disabled, so a repeat
// finishes whatever a crashed attempt left undone.
func (s *Service) disable(w http.ResponseWriter, r *http.Request, by store.Actor) {
	u, err := s.target(r, by)
	if err == nil {
		err = s.store.DisableUser(r.Context(), u.ID, by, "", s.clock.Now())
	}
	if err == nil && s.Instances != nil {
		err = s.Instances.Disable(r.Context(), u.ID)
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) enable(w http.ResponseWriter, r *http.Request, by store.Actor) {
	u, err := s.store.UserRowByID(r.Context(), r.PathValue("id"))
	if err == nil {
		err = s.store.EnableUser(r.Context(), u.ID, by, s.clock.Now())
	}
	if err != nil {
		fail(w, err)
		return
	}
	if s.Instances != nil {
		s.Instances.Ensure(u.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// delete removes the Instance and drains un-replayed data to the Admin Instance (which keeps
// it) before the User row goes, so a failure leaves a Disabled User whose Delete can be repeated.
func (s *Service) delete(w http.ResponseWriter, r *http.Request, by store.Actor) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	_ = httpx.Decode(w, r, 4096, &body)
	u, err := s.target(r, by)
	if err != nil {
		fail(w, err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(body.Confirm), u.Email) {
		http.Error(w, "type the User's email to confirm", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	err = s.store.BlockUser(ctx, u.ID, s.clock.Now())
	if err == nil && s.Instances != nil {
		err = s.Instances.Purge(ctx, u.ID)
	}
	if err == nil && s.Replay != nil {
		err = s.Replay.Drain(ctx, u.ID)
	}
	if err == nil {
		err = s.store.DeleteUser(ctx, u.ID, by, s.clock.Now())
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
