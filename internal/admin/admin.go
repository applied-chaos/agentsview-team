// Package admin is the Admin-only JSON API: Users, Admin grants and the Audit Log.
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/auth"
	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/hosts"
	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/instance"
	"github.com/applied-chaos/agentsview-team/internal/stats"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

// Instances reports each User's Instance state; ok is false when none is supervised.
type Instances interface {
	Status(userID string) (state instance.State, lastErr string, ok bool)
	AdminID() string
	Ensure(userID string)
	Stop(userID string)
	Disable(ctx context.Context, userID string) error
	Purge(ctx context.Context, userID string) error
}

// Replay finishes feeding a User's pending data to the Admin Instance.
type Replay interface {
	Drain(ctx context.Context, userID string) error
}

// Stats supplies the cached usage breakdown and space used.
type Stats interface {
	Users(ctx context.Context) (map[string]stats.UserUsage, error)
	Space(ctx context.Context) (*stats.Space, error)
}

type Service struct {
	auth  *auth.Service
	store *store.Store
	clock *clock.Clock
	// Instances is optional (nil: every Instance unknown).
	Instances Instances
	// Replay is optional (nil: nothing to drain before a Delete).
	Replay Replay
	// Stats is optional (nil: no figures).
	Stats Stats
}

func New(a *auth.Service, st *store.Store, clk *clock.Clock) *Service {
	return &Service{auth: a, store: st, clock: clk}
}

func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/users", s.admin(s.users))
	mux.HandleFunc("GET /api/admin/users/{id}", s.admin(s.user))
	mux.HandleFunc("PUT /api/admin/users/{id}/role", s.admin(s.setRole))
	mux.HandleFunc("POST /api/admin/users/{id}/disable", s.admin(s.disable))
	mux.HandleFunc("POST /api/admin/users/{id}/enable", s.admin(s.enable))
	mux.HandleFunc("POST /api/admin/users/{id}/delete", s.admin(s.delete))
	mux.HandleFunc("POST /api/admin/admins", s.admin(s.grant))
	mux.HandleFunc("GET /api/admin/audit", s.admin(s.audit))
	s.instanceRoutes(mux)
}

type adminHandler func(w http.ResponseWriter, r *http.Request, who store.Actor)

// admin checks the stored role, not the 15-minute token, so a revoked Admin loses access at once.
// An Admin who is impersonating acts as the User and so has no Admin access until they stop.
func (s *Service) admin(h adminHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, u, err := s.auth.Admin(r)
		if err == nil && p.Imp != "" {
			err = auth.ErrForbidden
		}
		if err != nil {
			auth.Reject(w, err)
			return
		}
		h(w, r, store.Actor{ID: &u.ID, Name: u.Name})
	}
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrLastAdmin):
		httpx.JSON(w, http.StatusConflict, map[string]string{"error": "last_admin"})
	case errors.Is(err, errSelf):
		httpx.JSON(w, http.StatusConflict, map[string]string{"error": "self"})
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		slog.Error("admin request failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

var errSelf = errors.New("cannot change your own access")

type instanceView struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type (
	Role       string
	UserStatus string
)

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"

	StatusActive         UserStatus = "active"
	StatusDisabled       UserStatus = "disabled"
	StatusInstanceFailed UserStatus = "instance_failed"
)

type userView struct {
	ID           *string    `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Role         Role       `json:"role"`
	PendingAdmin bool       `json:"pendingAdmin"`
	Status       UserStatus `json:"status"`
	Hosts        int        `json:"hosts"`
	StorageBytes int64      `json:"storageBytes"`
	LastActive   *time.Time `json:"lastActive"`
}

func (s *Service) toView(u store.UserRow) userView {
	v := userView{ID: &u.ID, Name: u.Name, Email: u.Email, Role: RoleUser, PendingAdmin: u.PendingAdmin, Status: StatusActive, LastActive: u.LastSignIn}
	if u.IsAdmin {
		v.Role = RoleAdmin
	}
	switch {
	case u.Disabled:
		v.Status = StatusDisabled
	case s.instanceState(u.ID) == instance.Failed:
		v.Status = StatusInstanceFailed
	}
	return v
}

func pendingView(p store.PendingAdmin) userView {
	return userView{Name: p.Email, Email: p.Email, Role: RoleUser, PendingAdmin: true, Status: StatusActive}
}

func (s *Service) instanceState(id string) instance.State {
	if s.Instances == nil {
		return ""
	}
	st, _, _ := s.Instances.Status(id)
	return st
}

func (s *Service) instance(id string) *instanceView {
	st := s.instanceState(id)
	switch st {
	case "":
		return nil
	case instance.Running, instance.Failed:
		return &instanceView{Status: string(st), Version: instance.PinnedVersion}
	default:
		return &instanceView{Status: "starting", Version: instance.PinnedVersion}
	}
}

func (s *Service) users(w http.ResponseWriter, r *http.Request, _ store.Actor) {
	rows, pending, err := s.store.ListUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	storage := map[string]int64{}
	if s.Stats != nil {
		if sp, err := s.Stats.Space(r.Context()); err == nil {
			storage = sp.Users
		}
	}
	out := make([]userView, 0, len(rows))
	for _, u := range rows {
		v := s.toView(u)
		v.StorageBytes = storage[u.ID]
		out = append(out, v)
	}
	for _, p := range pending {
		out = append(out, pendingView(p))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": out})
}

type userUsage struct {
	Sessions int64   `json:"sessions"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"costUsd"`
	Daily    []int64 `json:"daily"`
}

type hostRow struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	OS       string     `json:"os"`
	Online   bool       `json:"online"`
	LastSeen *time.Time `json:"lastSeen"`
}

type tokenRow struct {
	ID           string     `json:"id"`
	Description  string     `json:"description"`
	Reusable     bool       `json:"reusable"`
	Ephemeral    bool       `json:"ephemeral"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt"`
	HostsCreated int        `json:"hostsCreated"`
}

func (s *Service) user(w http.ResponseWriter, r *http.Request, _ store.Actor) {
	ctx := r.Context()
	u, err := s.store.UserRowByID(ctx, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	view := s.toView(u)
	usage := userUsage{Daily: make([]int64, 30)}
	var lastUpload *time.Time
	if s.Stats != nil {
		if sp, err := s.Stats.Space(ctx); err == nil {
			view.StorageBytes = sp.Users[u.ID]
		}
		if m, err := s.Stats.Users(ctx); err == nil {
			if row, ok := m[u.ID]; ok {
				usage = userUsage{Sessions: row.Sessions, Tokens: row.Tokens, CostUSD: row.Cost, Daily: make([]int64, len(row.Daily))}
				for i, d := range row.Daily {
					usage.Daily[i] = d.Tokens
				}
				lastUpload = row.LastActive
			}
		}
	}
	now := s.clock.Now()
	hostList := []hostRow{}
	hs, err := s.store.HostsOf(ctx, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	for _, h := range hs {
		if h.RevokedAt == nil {
			online := h.LastSeenAt != nil && now.Sub(*h.LastSeenAt) <= hosts.OnlineWithin
			hostList = append(hostList, hostRow{h.ID, h.Name, h.OS, online, h.LastSeenAt})
		}
	}
	view.Hosts = len(hostList)
	tokens := []tokenRow{}
	ts, err := s.store.InstallTokensOf(ctx, u.ID, now)
	if err != nil {
		fail(w, err)
		return
	}
	for _, t := range ts {
		tokens = append(tokens, tokenRow(t))
	}
	httpx.JSON(w, http.StatusOK, struct {
		userView
		Joined        time.Time     `json:"joined"`
		Instance      *instanceView `json:"instance"`
		HostList      []hostRow     `json:"hostList"`
		InstallTokens []tokenRow    `json:"installTokens"`
		Usage         userUsage     `json:"usage"`
		LastUpload    *time.Time    `json:"lastUpload"`
	}{view, u.CreatedAt, s.instance(u.ID), hostList, tokens, usage, lastUpload})
}

func (s *Service) setRole(w http.ResponseWriter, r *http.Request, by store.Actor) {
	var body struct {
		Admin *bool `json:"admin"`
	}
	if httpx.Decode(w, r, 4096, &body) != nil || body.Admin == nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	u, err := s.store.UserRowByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	if *body.Admin {
		_, err = s.store.GrantAdmin(r.Context(), u.Email, by, s.clock.Now())
	} else {
		err = s.store.RevokeAdmin(r.Context(), u.Email, by, s.clock.Now())
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) grant(w http.ResponseWriter, r *http.Request, by store.Actor) {
	var body struct {
		Email string `json:"email"`
	}
	if httpx.Decode(w, r, 4096, &body) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(body.Email))
	if err != nil || addr.Address != strings.TrimSpace(body.Email) {
		http.Error(w, "enter an email address", http.StatusBadRequest)
		return
	}
	pending, err := s.store.GrantAdmin(r.Context(), addr.Address, by, s.clock.Now())
	if err != nil {
		fail(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"pending": pending})
}

var ranges = map[string]time.Duration{"7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour}

const auditPageSize = 25

func (s *Service) audit(w http.ResponseWriter, r *http.Request, _ store.Actor) {
	q := r.URL.Query()
	query := store.AuditQuery{Text: strings.TrimSpace(q.Get("q")), Limit: auditPageSize}
	if t := q.Get("type"); t != "" && t != "all" {
		c, ok := store.ParseCategory(t)
		if !ok {
			http.Error(w, "unknown type", http.StatusBadRequest)
			return
		}
		query.Categories = []store.Category{c}
	}
	if d, ok := ranges[q.Get("range")]; ok {
		since := s.clock.Now().Add(-d)
		query.Since = &since
	}
	page, _ := strconv.Atoi(q.Get("page"))
	page = max(page, 1)
	query.Offset = (page - 1) * auditPageSize
	rows, total, err := s.store.ListAudit(r.Context(), query)
	if err != nil {
		fail(w, err)
		return
	}
	type entry struct {
		ID      int64     `json:"id"`
		At      time.Time `json:"at"`
		Actor   string    `json:"actor"`
		Action  string    `json:"action"`
		Target  string    `json:"target"`
		Details string    `json:"details"`
	}
	out := make([]entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, entry{r.ID, r.At, r.ActorName, r.Action.Label(), r.TargetLabel, r.Details})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"entries": out, "total": total, "page": page, "pageSize": auditPageSize})
}
