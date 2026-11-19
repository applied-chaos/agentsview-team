package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/store"
)

// Run audits the end of Impersonations whose session expired unrefreshed, until ctx ends.
func (s *Service) Run(ctx context.Context) {
	for t := time.NewTicker(time.Minute); ; {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.store.EndExpiredImpersonations(ctx, s.clock.Now()); err != nil {
				slog.Warn("end expired impersonations", "err", err)
			}
		}
	}
}

// ErrUnauthenticated and ErrForbidden are why Admin refused a request.
var (
	ErrUnauthenticated = errors.New("unauthorised")
	ErrForbidden       = errors.New("forbidden")
)

func Reject(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrUnauthenticated):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
	}
	http.Error(w, http.StatusText(status), status)
}

// Admin returns the enabled Admin behind the request, from the stored role so a revoked Admin
// loses access at once. Impersonation does not change who that is.
func (s *Service) Admin(r *http.Request) (AccessClaims, store.User, error) {
	p, ok := s.Principal(r)
	if !ok {
		return p, store.User{}, ErrUnauthenticated
	}
	u, err := s.store.UserByID(r.Context(), p.Subject)
	switch {
	case err != nil:
		return p, u, ErrUnauthenticated
	case !u.IsAdmin || u.Disabled:
		return p, u, ErrForbidden
	}
	return p, u, nil
}

// RecordOnBehalf audits an action taken while impersonating, as the Admin on the User's behalf.
func (s *Service) RecordOnBehalf(ctx context.Context, p AccessClaims, action store.Action, details string) error {
	if p.Imp == "" {
		return nil
	}
	admin, err := s.store.UserByID(ctx, p.Subject)
	if err != nil {
		return err
	}
	target, err := s.store.UserByID(ctx, p.Imp)
	if err != nil {
		return err
	}
	return s.store.RecordAudit(ctx, s.clock.Now(), store.AuditEntry{
		Actor: store.Actor{ID: &admin.ID, Name: admin.Name}, Action: action,
		TargetID: &target.ID, TargetLabel: target.Name, Details: details,
	})
}

func (s *Service) impersonate(w http.ResponseWriter, r *http.Request) {
	p, admin, err := s.Admin(r)
	if err != nil {
		Reject(w, err)
		return
	}
	target, err := s.store.UserByID(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		http.Error(w, "not found", http.StatusNotFound)
		return
	case target.ID == admin.ID:
		http.Error(w, "cannot impersonate yourself", http.StatusBadRequest)
		return
	case target.Disabled, p.Imp != "":
		http.Error(w, "conflict", http.StatusConflict)
		return
	}
	c, err := r.Cookie(cookieRefresh)
	if err != nil {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	by := store.Actor{ID: &admin.ID, Name: admin.Name}
	switch err := s.store.StartImpersonation(r.Context(), hashToken(c.Value), by, target, s.clock.Now()); {
	case errors.Is(err, store.ErrInvalidToken):
		http.Error(w, "conflict", http.StatusConflict)
		return
	case err != nil:
		slog.Error("start impersonation", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.setAccess(w, admin, target.ID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stopImpersonating is idempotent: stopping when not impersonating is a no-op.
func (s *Service) stopImpersonating(w http.ResponseWriter, r *http.Request) {
	p, ok := s.Principal(r)
	c, err := r.Cookie(cookieRefresh)
	if !ok || err != nil {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	if err := s.store.EndImpersonation(r.Context(), hashToken(c.Value), s.clock.Now()); err != nil {
		slog.Error("end impersonation", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	admin, err := s.store.UserByID(r.Context(), p.Subject)
	if err != nil {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	if err := s.setAccess(w, admin, ""); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
