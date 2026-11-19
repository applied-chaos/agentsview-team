// Package auth is the Platform's Identity module: OIDC sign-in, the sign-in rule,
// 15-minute access JWTs and rotating refresh tokens in cookies.
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/config"
	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

const (
	cookieAccess  = "av_access"
	cookieRefresh = "av_refresh"
	cookieFlow    = "av_oidc"
	cookieDenied  = "av_denied"
)

type Service struct {
	cfg      config.Config
	rule     Rule
	store    *store.Store
	clock    *clock.Clock
	signer   signer
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier

	// OnSignIn runs after a permitted, enabled User signs in (first sign-in provisions their Instance).
	OnSignIn func(store.User)
	// OnDisabled stops the Instance and revokes device credentials after an auto-Disable.
	OnDisabled func(ctx context.Context, userID string)
}

// New discovers the OIDC provider, retrying while it starts.
func New(ctx context.Context, cfg config.Config, st *store.Store, clk *clock.Clock) (*Service, error) {
	var (
		provider *oidc.Provider
		err      error
	)
	deadline := time.Now().Add(60 * time.Second)
	for {
		if provider, err = oidc.NewProvider(ctx, cfg.OIDC.Issuer); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("oidc discovery: %w", err)
		}
		time.Sleep(time.Second)
	}
	return &Service{
		cfg: cfg, store: st, clock: clk,
		rule:   Rule{Domains: cfg.AllowedDomains, Group: cfg.RequiredGroup},
		signer: newSigner(cfg.SecretKey, clk.Now),
		oauth: oauth2.Config{
			ClientID: cfg.OIDC.ClientID, ClientSecret: cfg.OIDC.ClientSecret, Endpoint: provider.Endpoint(),
			RedirectURL: cfg.BaseURL + "/auth/callback", Scopes: cfg.OIDC.Scopes,
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.OIDC.ClientID, SupportedSigningAlgs: asymmetricAlgs}),
	}, nil
}

// Routes registers /auth/* and the identity JSON API.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", s.login)
	mux.HandleFunc("GET /auth/callback", s.callback)
	mux.HandleFunc("POST /auth/refresh", s.refresh)
	mux.HandleFunc("POST /auth/logout", s.logout)
	mux.HandleFunc("POST /auth/impersonate/stop", s.stopImpersonating)
	mux.HandleFunc("POST /auth/impersonate/{id}", s.impersonate)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/public", s.public)
	mux.HandleFunc("GET /api/denied", s.denied)
}

func (s *Service) setCookie(w http.ResponseWriter, name, value, path string, ttl time.Duration) {
	c := &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())}
	if value == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func (s *Service) clearSession(w http.ResponseWriter) {
	s.setCookie(w, cookieAccess, "", "/", 0)
	s.setCookie(w, cookieRefresh, "", "/auth", 0)
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	state, err1 := randomToken()
	nonce, err2 := randomToken()
	if err := errors.Join(err1, err2); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()
	now := s.clock.Now()
	flow, err := s.signer.sign(flowClaims{State: state, Nonce: nonce, Verifier: verifier, RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(now.Add(flowTTL)),
	}})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, cookieFlow, flow, "/auth", flowTTL)
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (s *Service) callback(w http.ResponseWriter, r *http.Request) {
	fc, err := r.Cookie(cookieFlow)
	if err != nil {
		httpx.Page(w, http.StatusBadRequest, "sign-in-expired", "sign-in expired, start again")
		return
	}
	var flow flowClaims
	if err := s.signer.parse(fc.Value, &flow); err != nil || flow.State == "" || r.URL.Query().Get("state") != flow.State {
		slog.Warn("sign-in state mismatch or unreadable flow cookie", "err", err)
		httpx.Page(w, http.StatusBadRequest, "sign-in-expired", "invalid sign-in state")
		return
	}
	s.setCookie(w, cookieFlow, "", "/auth", 0)
	if e := r.URL.Query().Get("error"); e != "" {
		slog.Warn("identity provider refused sign-in", "error", e, "description", r.URL.Query().Get("error_description"))
		httpx.Page(w, http.StatusBadRequest, "provider-refused", "identity provider refused sign-in")
		return
	}
	tok, err := s.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		attrs := []any{"err", err}
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			attrs = append(attrs, "status", re.Response.StatusCode, "error", re.ErrorCode, "description", re.ErrorDescription)
		}
		slog.Warn("code exchange failed", attrs...)
		httpx.Page(w, http.StatusBadGateway, "provider-unavailable", "sign-in failed")
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	claims, err := s.verifyIDToken(r.Context(), raw, flow.Nonce)
	if err != nil {
		alg, kid := tokenHeader(raw)
		slog.Warn("id token rejected", "err", err, "issuer", s.cfg.OIDC.Issuer, "alg", alg, "kid", kid)
		httpx.Page(w, http.StatusBadRequest, "bad-identity", "invalid identity token")
		return
	}
	email, _ := claims["email"].(string)
	email = strings.ToLower(strings.TrimSpace(email))
	if v, ok := claims["email_verified"].(bool); email == "" || !ok || !v {
		slog.Warn("identity provider gave no verified email", "issuer", s.cfg.OIDC.Issuer, "email_present", email != "", "email_verified_claim", claims["email_verified"])
		httpx.Page(w, http.StatusBadRequest, "no-verified-email", "identity provider gave no verified email")
		return
	}
	name, _ := claims["name"].(string)
	if name = strings.TrimSpace(name); name == "" {
		name, _, _ = strings.Cut(email, "@")
	}

	if !s.rule.Permits(email, stringList(claims[s.cfg.OIDC.GroupsClaim])) {
		id, err := s.store.DisableUserByEmail(r.Context(), email, s.clock.Now())
		if err != nil {
			slog.Error("disable user", "err", err)
		} else if id != "" && s.OnDisabled != nil {
			s.OnDisabled(r.Context(), id)
		}
		s.deny(w, r, email)
		return
	}
	user, err := s.store.UpsertUser(r.Context(), email, name)
	if err != nil {
		slog.Error("upsert user", "err", err)
		httpx.Page(w, http.StatusInternalServerError, "server-error", "internal error")
		return
	}
	if user.Disabled {
		s.deny(w, r, email)
		return
	}
	if s.OnSignIn != nil {
		s.OnSignIn(user)
	}
	refreshToken, err := randomToken()
	if err == nil {
		now := s.clock.Now()
		err = s.store.NewRefreshChain(r.Context(), user.ID, hashToken(refreshToken), now, now.Add(RefreshTTL))
	}
	if err != nil {
		slog.Error("start session", "err", err)
		httpx.Page(w, http.StatusInternalServerError, "server-error", "internal error")
		return
	}
	if err := s.setSession(w, user, refreshToken, ""); err != nil {
		httpx.Page(w, http.StatusInternalServerError, "server-error", "internal error")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Service) setSession(w http.ResponseWriter, u store.User, refreshToken, imp string) error {
	if err := s.setAccess(w, u, imp); err != nil {
		return err
	}
	s.setCookie(w, cookieRefresh, refreshToken, "/auth", RefreshTTL)
	return nil
}

func (s *Service) setAccess(w http.ResponseWriter, u store.User, imp string) error {
	access, err := s.signer.access(u.ID, u.IsAdmin, imp)
	if err != nil {
		return err
	}
	s.setCookie(w, cookieAccess, access, "/", AccessTTL)
	return nil
}

func (s *Service) deny(w http.ResponseWriter, r *http.Request, email string) {
	s.clearSession(w)
	if v, err := s.signer.sign(deniedClaims{Email: email, RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(s.clock.Now().Add(flowTTL)),
	}}); err == nil {
		s.setCookie(w, cookieDenied, v, "/", flowTTL)
	}
	http.Redirect(w, r, "/not-permitted", http.StatusFound)
}

func (s *Service) refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieRefresh)
	if err != nil {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	next, err := randomToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	now := s.clock.Now()
	user, imp, err := s.store.RotateRefresh(r.Context(), hashToken(c.Value), hashToken(next), now, now.Add(RefreshTTL))
	switch {
	case errors.Is(err, store.ErrInvalidToken), errors.Is(err, store.ErrTokenReuse):
		if errors.Is(err, store.ErrTokenReuse) {
			slog.Warn("refresh token reused; chain revoked")
		}
		s.clearSession(w)
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	case err != nil:
		slog.Error("rotate refresh", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.setSession(w, user, next, imp); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieRefresh); err == nil {
		// A forgotten Impersonation still gets its audited end.
		if err := s.store.EndImpersonation(r.Context(), hashToken(c.Value), s.clock.Now()); err != nil {
			slog.Error("end impersonation", "err", err)
		}
		if err := s.store.RevokeChain(r.Context(), hashToken(c.Value), s.clock.Now()); err != nil {
			slog.Error("revoke chain", "err", err)
		}
	}
	s.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

// Principal is the signed-in caller, from a valid access token.
func (s *Service) Principal(r *http.Request) (AccessClaims, bool) {
	c, err := r.Cookie(cookieAccess)
	if err != nil {
		return AccessClaims{}, false
	}
	claims, err := s.signer.verifyAccess(c.Value)
	return claims, err == nil
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	p, ok := s.Principal(r)
	if !ok {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	u, err := s.store.UserByID(r.Context(), p.Effective())
	if err != nil {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	out := map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "isAdmin": u.IsAdmin}
	if p.Imp != "" {
		if by, err := s.store.UserByID(r.Context(), p.Subject); err == nil {
			out["impersonatedBy"] = map[string]any{"id": by.ID, "name": by.Name, "email": by.Email}
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Service) public(w http.ResponseWriter, _ *http.Request) {
	domains := s.cfg.AllowedDomains
	if domains == nil {
		domains = []string{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"providerName": s.cfg.OIDC.ProviderName, "domains": domains, "group": s.cfg.RequiredGroup})
}

func (s *Service) denied(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieDenied)
	var d deniedClaims
	if err != nil || s.signer.parse(c.Value, &d) != nil {
		http.Error(w, "nothing to show", http.StatusNotFound)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"email": d.Email})
}

func stringList(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// asymmetricAlgs is every signature algorithm go-oidc supports, whatever discovery said at boot:
// a provider can change its signing key while the Platform runs. Keys come from the live JWKS.
var asymmetricAlgs = []string{
	oidc.RS256, oidc.RS384, oidc.RS512, oidc.ES256, oidc.ES384, oidc.ES512,
	oidc.PS256, oidc.PS384, oidc.PS512, oidc.EdDSA,
}

func tokenHeader(raw string) (alg, kid string) {
	head, _, _ := strings.Cut(raw, ".")
	b, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		return "unreadable", ""
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(b, &h) != nil || h.Alg == "" {
		return "unreadable", ""
	}
	return h.Alg, h.Kid
}

func (s *Service) verifyIDToken(ctx context.Context, raw, nonce string) (map[string]any, error) {
	if alg, _ := tokenHeader(raw); alg == "HS256" {
		claims, err := verifyHS256(raw, s.cfg.OIDC.ClientSecret, s.cfg.OIDC.Issuer, s.cfg.OIDC.ClientID, time.Now())
		if err != nil {
			return nil, err
		}
		if got, _ := claims["nonce"].(string); got != nonce {
			return nil, errors.New("nonce mismatch")
		}
		return claims, nil
	}
	idt, err := s.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	if idt.Nonce != nonce {
		return nil, errors.New("nonce mismatch")
	}
	claims := map[string]any{}
	if err := idt.Claims(&claims); err != nil {
		return nil, fmt.Errorf("claims: %w", err)
	}
	return claims, nil
}
