// Package hosts adds Hosts to a User's Instance: install tokens, enrolment, the install
// script and the My Hosts API.
package hosts

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/auth"
	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/store"
	"github.com/applied-chaos/agentsview-team/internal/tokens"
)

const (
	TokenTTL = time.Hour
	// OnlineWithin is how recently a Host must have been seen to count as Online.
	OnlineWithin = 30 * time.Minute
	// EphemeralTTL is how long an Ephemeral Host may go unseen before it is revoked.
	EphemeralTTL = 30 * 24 * time.Hour
)

// Devices creates raw-sync devices in a User's Instance.
type Devices interface {
	EnrolDevice(ctx context.Context, userID, displayName string) (deviceID, credential string, err error)
	RenameDevice(ctx context.Context, userID, deviceID, displayName string) error
	RevokeDevice(ctx context.Context, userID, deviceID string) error
	CredentialValid(ctx context.Context, userID, deviceID, credential string) (bool, error)
}

// Authn is the Identity module as this package needs it.
type Authn interface {
	Principal(r *http.Request) (auth.AccessClaims, bool)
	RecordOnBehalf(ctx context.Context, p auth.AccessClaims, action store.Action, details string) error
}

type Service struct {
	st        *store.Store
	dev       Devices
	clk       *clock.Clock
	authn     Authn
	instances Instances
	baseURL   string
	version   string
	OnRename  RenameHook
	usage     usageCache
	// Space reports a User's space used in bytes (nil: not shown).
	Space func(ctx context.Context, userID string) (int64, error)
}

func New(st *store.Store, dev Devices, inst Instances, clk *clock.Clock, a Authn, baseURL, version string) *Service {
	return &Service{st: st, dev: dev, instances: inst, clk: clk, authn: a, baseURL: baseURL, version: version,
		usage: usageCache{entries: map[usageKey]usageEntry{}}}
}

func (s *Service) Routes(mux *http.ServeMux) {
	s.scriptRoutes(mux)
	mux.HandleFunc("POST /api/hosts/enrol", s.enrol)
	mux.HandleFunc("POST /api/hosts/tokens", s.newToken)
	mux.HandleFunc("GET /api/hosts/tokens", s.tokens)
	mux.HandleFunc("GET /api/hosts/tokens/{id}", s.tokenState)
	mux.HandleFunc("POST /api/hosts/tokens/{id}/revoke", s.revokeToken)
	mux.HandleFunc("GET /api/hosts", s.list)
	mux.HandleFunc("GET /api/hosts/{id}", s.detail)
	mux.HandleFunc("PATCH /api/hosts/{id}", s.rename)
	mux.HandleFunc("POST /api/hosts/{id}/revoke", s.revoke)
	mux.HandleFunc("DELETE /api/hosts/{id}", s.delete)
	mux.HandleFunc("POST /api/hosts/checkin", s.checkin)
	mux.HandleFunc("GET /api/overview", s.overview)
}

// Seen and Uploaded implement instance.Observer.
func (s *Service) Seen(ctx context.Context, deviceID string) {
	if err := s.st.TouchHostSeen(ctx, deviceID, s.clk.Now()); err != nil {
		slog.Warn("record host seen", "err", err)
	}
}

func (s *Service) Uploaded(ctx context.Context, deviceID string) {
	if err := s.st.TouchHostUpload(ctx, deviceID, s.clk.Now()); err != nil {
		slog.Warn("record host upload", "err", err)
	}
}

// user is the signed-in, enabled caller; it writes the 401 itself when there is none.
func (s *Service) user(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	claims, ok := s.authn.Principal(r)
	if ok {
		if u, err := s.st.UserByID(r.Context(), claims.Effective()); err == nil && !u.Disabled {
			return u, true
		}
	}
	http.Error(w, "unauthorised", http.StatusUnauthorized)
	return store.User{}, false
}

type Expiry string

// expiries maps each lifetime to its duration; zero is Never.
var expiries = map[Expiry]time.Duration{
	"1h": time.Hour, "1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "never": 0,
}

// A token with no stated expiry lives as the form defaults it, or an hour when single-use (Add Host).
const (
	defaultExpiry   Expiry = "7d"
	singleUseExpiry Expiry = "1h"
)

const badTokenRequest = "description up to 100 characters; expiry 1 hour, 1 day, 7 days, 30 days or never"

// tokenRequest is the optional body of POST /api/hosts/tokens; with no body the token is
// the single-use, one-hour kind the Add Host page uses.
type tokenRequest struct {
	Description string `json:"description"`
	Reusable    bool   `json:"reusable"`
	Ephemeral   bool   `json:"ephemeral"`
	ExpiresIn   Expiry `json:"expiresIn"`
}

func (s *Service) newToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	ttl, opts := TokenTTL, store.TokenOpts{}
	var q tokenRequest
	switch err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); {
	case errors.Is(err, io.EOF):
	case err != nil || len([]rune(q.Description)) > 100:
		httpx.Fail(w, http.StatusBadRequest, badTokenRequest)
		return
	default:
		if q.ExpiresIn == "" {
			q.ExpiresIn = defaultExpiry
			if !q.Reusable {
				q.ExpiresIn = singleUseExpiry
			}
		}
		var known bool
		if ttl, known = expiries[q.ExpiresIn]; !known {
			httpx.Fail(w, http.StatusBadRequest, badTokenRequest)
			return
		}
		opts = store.TokenOpts{Description: strings.TrimSpace(q.Description), Reusable: q.Reusable, Ephemeral: q.Ephemeral}
	}
	if !s.auditOnBehalf(w, r, store.ActionImpersonatedTokenNew, cmp.Or(opts.Description, "no description")) {
		return
	}
	secret, err := tokens.Random(24)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not create token")
		return
	}
	token := "avt_tk_" + secret
	now := s.clk.Now()
	var expires *time.Time
	if ttl > 0 {
		expires = new(now.Add(ttl))
	}
	id, err := s.st.CreateInstallToken(r.Context(), u.ID, tokens.Hash(token), now, expires, opts)
	if err != nil {
		slog.Error("create install token", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "could not create token")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id": id, "token": token, "createdAt": now, "expiresAt": expires, "baseUrl": s.baseURL,
		"description": opts.Description, "reusable": opts.Reusable, "ephemeral": opts.Ephemeral,
	})
}

func (s *Service) tokens(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	list, err := s.st.InstallTokensOf(r.Context(), u.ID, s.clk.Now())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not list tokens")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, t := range list {
		out = append(out, map[string]any{
			"id": t.ID, "description": t.Description, "reusable": t.Reusable, "ephemeral": t.Ephemeral,
			"createdAt": t.CreatedAt, "expiresAt": t.ExpiresAt, "lastUsedAt": t.LastUsedAt, "hostsCreated": t.HostsCreated,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Service) revokeToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	if !s.auditOnBehalf(w, r, store.ActionImpersonatedTokenStop, r.PathValue("id")) {
		return
	}
	err := s.st.RevokeInstallToken(r.Context(), u.ID, r.PathValue("id"), s.clk.Now())
	if errors.Is(err, store.ErrInstallToken) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not revoke token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RevokeStale revokes everyone's live Ephemeral Hosts unseen for EphemeralTTL: the Instance
// device first, so a crash between the two steps retries.
func (s *Service) RevokeStale(ctx context.Context) error {
	now := s.clk.Now()
	stale, err := s.st.StaleEphemeral(ctx, now.Add(-EphemeralTTL))
	if err != nil {
		return err
	}
	var errs []error
	for _, h := range stale {
		if err := s.dev.RevokeDevice(ctx, h.UserID, h.DeviceID); err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, s.st.RevokeHost(ctx, h.ID, now))
	}
	return errors.Join(errs...)
}

func (s *Service) Run(ctx context.Context) {
	for t := time.NewTicker(sweepEvery); ; {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.RevokeStale(ctx); err != nil {
				slog.Warn("revoke stale Ephemeral Hosts", "err", err)
			}
		}
	}
}

func (s *Service) tokenState(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	state, host, err := s.st.TokenHost(r.Context(), u.ID, r.PathValue("id"), s.clk.Now())
	if errors.Is(err, store.ErrInstallToken) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not read token")
		return
	}
	out := map[string]any{"state": state}
	if host != nil {
		out["host"] = s.view(*host)
	}
	httpx.JSON(w, http.StatusOK, out)
}

type HostStatus string

const (
	StatusOnline  HostStatus = "online"
	StatusOffline HostStatus = "offline"
	StatusRevoked HostStatus = "revoked"
)

type hostView struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	OS           string            `json:"os"`
	InstallMode  store.InstallMode `json:"installMode"`
	Status       HostStatus        `json:"status"`
	LastSeenAt   *time.Time        `json:"lastSeenAt"`
	LastUploadAt *time.Time        `json:"lastUploadAt"`
	RevokedAt    *time.Time        `json:"revokedAt"`
	CreatedAt    time.Time         `json:"createdAt"`
	Version      string            `json:"version"`
	Outdated     bool              `json:"outdated"`
	Ephemeral    bool              `json:"ephemeral"`
}

func (s *Service) view(h store.Host) hostView {
	status := StatusOffline
	switch {
	case h.RevokedAt != nil:
		status = StatusRevoked
	case h.LastSeenAt != nil && s.clk.Now().Sub(*h.LastSeenAt) <= OnlineWithin:
		status = StatusOnline
	}
	return hostView{
		ID: h.ID, Name: h.Name, OS: h.OS, InstallMode: h.InstallMode, Status: status,
		LastSeenAt: h.LastSeenAt, LastUploadAt: h.LastUploadAt, RevokedAt: h.RevokedAt, CreatedAt: h.CreatedAt,
		Version: h.ClientVersion, Outdated: h.ClientVersion != s.version, Ephemeral: h.Ephemeral,
	}
}

func (s *Service) hostList(ctx context.Context, userID string) ([]hostView, error) {
	hosts, err := s.st.HostsOf(ctx, userID)
	out := make([]hostView, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, s.view(h))
	}
	return out, err
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	hosts, err := s.hostList(r.Context(), u.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not list Hosts")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hosts": hosts, "pinnedVersion": s.version})
}

var (
	nameRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	osRe      = regexp.MustCompile(`^[A-Za-z0-9 ._()/-]{1,64}$`)
	versionRe = regexp.MustCompile(`^[0-9A-Za-z._+-]{1,32}$`)
)

type enrolRequest struct {
	Token   string            `json:"token"`
	Name    string            `json:"name"`
	OS      string            `json:"os"`
	Mode    store.InstallMode `json:"mode"`
	Version string            `json:"version"`
}

func (q enrolRequest) valid() bool {
	_, knownMode := installModes[q.Mode]
	return nameRe.MatchString(q.Name) && osRe.MatchString(q.OS) && versionRe.MatchString(q.Version) && knownMode
}

// enrol redeems an Install Token: it creates the Host's device in the User's Instance and
// returns the Credential, once. The install script is the only caller.
func (s *Service) enrol(w http.ResponseWriter, r *http.Request) {
	var q enrolRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); err != nil || !q.valid() {
		httpx.Fail(w, http.StatusBadRequest, "malformed enrolment request")
		return
	}
	ctx := r.Context()
	tokenID, userID, err := s.st.ClaimInstallToken(ctx, tokens.Hash(q.Token), s.clk.Now())
	if errors.Is(err, store.ErrInstallToken) {
		httpx.Fail(w, http.StatusUnauthorized, "this Install Token is invalid, used or expired; add the Host again for a new command")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not use token")
		return
	}
	var credential string
	host, err := s.st.CreateHost(ctx, userID, tokenID,
		store.NewHost{Name: q.Name, OS: q.OS, InstallMode: q.Mode, ClientVersion: q.Version}, s.clk.Now(),
		func(name string) (string, error) {
			dev, cred, err := s.dev.EnrolDevice(ctx, userID, name)
			credential = cred
			return dev, err
		})
	if err != nil {
		slog.Error("enrol host", "user", userID, "err", err)
		if rerr := s.st.ReleaseInstallToken(context.WithoutCancel(ctx), tokenID); rerr != nil {
			slog.Error("release install token", "err", rerr)
		}
		httpx.Fail(w, http.StatusServiceUnavailable, "your Instance is not ready yet; run the command again in a minute")
		return
	}
	userPath := "/i/" + userID
	if strings.Contains(r.Header.Get("Accept"), "text/x-env") {
		w.Header().Set("Content-Type", "text/x-env; charset=utf-8")
		_, _ = fmt.Fprintf(w, "AV_HOST_NAME=%s\nAV_USER_PATH=%s\nAGENTSVIEW_RAW_SYNC_DEVICE_ID=%s\nAGENTSVIEW_RAW_SYNC_CREDENTIAL=%s\n",
			host.Name, userPath, host.DeviceID, credential)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{
		"name": host.Name, "userPath": userPath, "deviceId": host.DeviceID, "credential": credential,
	})
}
