package stats

import (
	"context"
	"log/slog"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/store"
)

// Run refreshes at once, then every RefreshEvery (or retryEvery after a failure), until ctx ends.
func (s *Service) Run(ctx context.Context) {
	loop(ctx, s.Refresh, RefreshEvery, retryEvery)
}

func loop(ctx context.Context, refresh func(context.Context) bool, every, retry time.Duration) {
	for {
		wait := every
		if !refresh(ctx) {
			wait = retry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Refresh recomputes the org figures, every User's own figures and the space snapshot. A piece
// that fails keeps its last good value. It reports whether the org figures and space loaded; one
// User's Instance being down does not count, as stopped Instances are normal.
func (s *Service) Refresh(ctx context.Context) bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	ok := true
	if d, err := s.loadOrg(ctx); err != nil {
		slog.Error("stats: org figures", "err", err)
		ok = false
	} else {
		s.mu.Lock()
		s.orgCache = d
		s.mu.Unlock()
	}
	if users, err := s.st.StatsUsers(ctx); err != nil {
		slog.Error("stats: list users", "err", err)
	} else {
		_ = forEach(users, func(u store.StatsUser) error {
			f, err := s.loadOwn(ctx, u.ID)
			if err != nil {
				slog.Error("stats: user figures", "user", u.ID, "err", err)
				return nil
			}
			s.mu.Lock()
			s.own[u.ID] = f
			s.mu.Unlock()
			return nil
		})
	}
	if sp, err := s.computeSpace(ctx); err != nil {
		slog.Error("stats: space", "err", err)
		ok = false
	} else {
		s.mu.Lock()
		s.space = sp
		s.mu.Unlock()
	}
	return ok
}
