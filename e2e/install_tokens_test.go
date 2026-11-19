//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type tokenOpts struct {
	Description string `json:"description"`
	Reusable    bool   `json:"reusable"`
	Ephemeral   bool   `json:"ephemeral"`
	ExpiresIn   string `json:"expiresIn,omitempty"`
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
	Token        string     `json:"token"`
}

func (b *browser) createToken(o tokenOpts) *http.Response {
	b.t.Helper()
	payload, _ := json.Marshal(o)
	return b.do("POST", platformURL+"/api/hosts/tokens", strings.NewReader(string(payload)))
}

func (b *browser) mustCreateToken(o tokenOpts) tokenRow {
	b.t.Helper()
	resp := b.createToken(o)
	if resp.StatusCode != http.StatusCreated {
		b.t.Fatalf("create token: %d %s", resp.StatusCode, body(b.t, resp))
	}
	return decode[tokenRow](b.t, resp)
}

func (b *browser) tokenRows() []tokenRow {
	b.t.Helper()
	resp := b.get("/api/hosts/tokens")
	if resp.StatusCode != http.StatusOK {
		b.t.Fatalf("list tokens: %d %s", resp.StatusCode, body(b.t, resp))
	}
	return decode[struct {
		Tokens []tokenRow `json:"tokens"`
	}](b.t, resp).Tokens
}

func (b *browser) tokenRow(id string) *tokenRow {
	for _, r := range b.tokenRows() {
		if r.ID == id {
			return &r
		}
	}
	return nil
}

func TestInstallTokens(t *testing.T) {
	t.Parallel()
	f := newFx(t, "tokens")
	f.run(t, "expiry options and secret shown once", tokensExpiryOptions)
	f.run(t, "reusable names hosts by hostname", tokensReusable)
	f.run(t, "revoked token enrols nothing", tokensRevoked)
	f.run(t, "ephemeral tick", tokensEphemeralTick)
	f.run(t, "credential and token responses not cacheable", tokensNoStore)
}

func tokensNoStore(t *testing.T, f *fx) {
	resp := f.b.createToken(tokenOpts{Description: "cache"})
	resp.Body.Close()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Install Token response Cache-Control %q", got)
	}
	tok := f.b.mustCreateToken(tokenOpts{Description: "enrol"})
	resp = enrolRequest(t, tok.Token, "nostore-box")
	resp.Body.Close()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("enrol response Cache-Control %q", got)
	}
}

func tokensExpiryOptions(t *testing.T, f *fx) {
	b := f.b
	for _, e := range []string{"2d", "90d", "0", "forever"} {
		if code := statusOf(b.createToken(tokenOpts{Description: "x", ExpiresIn: e})); code != http.StatusBadRequest {
			t.Fatalf("expiry %q: %d, want 400", e, code)
		}
	}
	long := b.mustCreateToken(tokenOpts{Description: "CI runners", Reusable: true, Ephemeral: true})
	if !strings.HasPrefix(long.Token, "avt_tk_") {
		t.Fatalf("token %q", long.Token)
	}
	if got := long.ExpiresAt.Sub(long.CreatedAt); got != 7*24*time.Hour {
		t.Fatalf("default expiry %v, want 7 days", got)
	}
	for e, want := range map[string]time.Duration{"1h": time.Hour, "1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour} {
		tok := b.mustCreateToken(tokenOpts{Description: e, ExpiresIn: e})
		if got := tok.ExpiresAt.Sub(tok.CreatedAt); got != want {
			t.Fatalf("expiry %s: %v, want %v", e, got, want)
		}
	}
	never := b.mustCreateToken(tokenOpts{Description: "never", Reusable: true, ExpiresIn: "never"})
	if never.ExpiresAt != nil {
		t.Fatalf("Never token expires at %v", never.ExpiresAt)
	}
	if row := b.tokenRow(never.ID); row == nil || row.ExpiresAt != nil {
		t.Fatalf("listed Never token %+v", row)
	}
	mustEnrol(t, never.Token, "forever-box")
	row := b.tokenRow(long.ID)
	if row == nil || row.Token != "" || row.Description != "CI runners" || !row.Reusable || !row.Ephemeral || row.HostsCreated != 0 || row.LastUsedAt != nil {
		t.Fatalf("listed token %+v", row)
	}
	if n := count(t, `select count(*) from install_tokens where token_hash = sha256(convert_to($1, 'UTF8'))`, long.Token); n != 1 {
		t.Fatal("token is not stored as a hash")
	}
	if code := statusOf(newBrowser(t).get("/api/hosts/tokens")); code != http.StatusUnauthorized {
		t.Fatalf("signed out list: %d", code)
	}
	other := plainUser.browser(t)
	if len(other.tokenRows()) != 0 {
		t.Fatal("another User's tokens are listed")
	}
}

func tokensReusable(t *testing.T, f *fx) {
	b := f.b
	tok := b.mustCreateToken(tokenOpts{Description: "fleet", Reusable: true})
	var names []string
	for range 3 {
		names = append(names, mustEnrol(t, tok.Token, "tok-runner").Name)
	}
	if fmt.Sprint(names) != "[tok-runner tok-runner-2 tok-runner-3]" {
		t.Fatalf("names %v", names)
	}
	row := b.tokenRow(tok.ID)
	if row == nil || row.HostsCreated != 3 || row.LastUsedAt == nil {
		t.Fatalf("usage %+v", row)
	}
	single := b.mustCreateToken(tokenOpts{Description: "once"})
	mustEnrol(t, single.Token, "solo")
	if code := statusOf(enrolRequest(t, single.Token, "solo2")); code != http.StatusUnauthorized {
		t.Fatalf("single-use token reused: %d", code)
	}
	if row := b.tokenRow(single.ID); row != nil && row.HostsCreated != 1 {
		t.Fatalf("single-use usage %+v", row)
	}
}

func tokensRevoked(t *testing.T, f *fx) {
	b := f.b
	tok := b.mustCreateToken(tokenOpts{Description: "temp", Reusable: true})
	e := mustEnrol(t, tok.Token, "tok-keeper")
	other := plainUser.browser(t)
	if code := statusOf(other.post("/api/hosts/tokens/" + tok.ID + "/revoke")); code != http.StatusNotFound {
		t.Fatalf("another User revoking: %d, want 404", code)
	}
	if code := statusOf(b.post("/api/hosts/tokens/" + tok.ID + "/revoke")); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if code := statusOf(b.post("/api/hosts/tokens/" + tok.ID + "/revoke")); code != http.StatusNoContent {
		t.Fatalf("second revoke: %d, want idempotent 204", code)
	}
	if code := statusOf(enrolRequest(t, tok.Token, "latecomer")); code != http.StatusUnauthorized {
		t.Fatalf("revoked token enrolled: %d, want 401", code)
	}
	if b.tokenRow(tok.ID) != nil {
		t.Fatal("revoked token still listed")
	}
	if state, _ := b.tokenState(tok.ID); state != "revoked" {
		t.Fatalf("state %q", state)
	}
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusOK {
		t.Fatalf("existing Host after token revoke: %d, want 200", code)
	}
	if h := b.hostsNamed("tok-keeper"); len(h) != 1 || h[0].Status == "revoked" {
		t.Fatalf("Hosts %+v", h)
	}
}

func TestInstallTokenExpiresOnTheTestClock(t *testing.T) {
	b, _, _ := running(t, "tokclock")
	tok := b.mustCreateToken(tokenOpts{Description: "day", Reusable: true, ExpiresIn: "1d"})
	advance(t, 23*time.Hour)
	mustEnrol(t, tok.Token, "early")
	advance(t, 2*time.Hour)
	if code := statusOf(enrolRequest(t, tok.Token, "late")); code != http.StatusUnauthorized {
		t.Fatalf("expired token: %d, want 401", code)
	}
	b.signIn()
	if b.tokenRow(tok.ID) != nil {
		t.Fatal("expired token still listed")
	}
}

func TestEphemeralHostsAreRevokedAfterThirtyDaysUnseen(t *testing.T) {
	b, _, _ := running(t, "ephemeral")
	m, _ := b.me()
	eph := b.mustCreateToken(tokenOpts{Description: "ci", Reusable: true, Ephemeral: true, ExpiresIn: "never"})
	keep := b.mustCreateToken(tokenOpts{Description: "laptops", Reusable: true, ExpiresIn: "never"})
	stale := mustEnrol(t, eph.Token, "stale")
	active := mustEnrol(t, eph.Token, "active")
	durable := mustEnrol(t, keep.Token, "durable")

	view := func() map[string]hostView {
		b.signIn()
		out := map[string]hostView{}
		for _, h := range b.hosts().Hosts {
			out[h.Name] = h
		}
		return out
	}
	hosts := view()
	if !hosts["stale"].Ephemeral || !hosts["active"].Ephemeral || hosts["durable"].Ephemeral {
		t.Fatalf("ephemeral flags %+v", hosts)
	}

	advance(t, 20*24*time.Hour)
	resp := rawSyncToken(t, active.UserPath, active)
	resp.Body.Close()
	advance(t, 9*24*time.Hour)
	if got := view(); got["stale"].Status == "revoked" {
		t.Fatalf("revoked after 29 days: %+v", got["stale"])
	}
	advance(t, 2*24*time.Hour)
	eventually(t, "the sweep revokes the stale Host", 30*time.Second, func() bool { return view()["stale"].Status == "revoked" })
	got := view()
	if got["stale"].Status != "revoked" || got["stale"].RevokedAt == nil {
		t.Fatalf("stale Ephemeral Host after 31 days: %+v", got["stale"])
	}
	if got["active"].Status == "revoked" || got["durable"].Status == "revoked" {
		t.Fatalf("revoked too much: %+v", got)
	}
	if n := instanceCount(t, m.ID, `select count(*) from agentsview.raw_devices where device_id = $1 and revoked_at is not null`, stale.DeviceID); n != 1 {
		t.Fatal("device not revoked in the Instance")
	}
	if code := statusOf(rawSyncToken(t, stale.UserPath, stale)); code == http.StatusOK {
		t.Fatal("revoked Host's Credential still accepted")
	}
	if code := statusOf(rawSyncToken(t, durable.UserPath, durable)); code != http.StatusOK {
		t.Fatalf("durable Host: %d", code)
	}
	if mustEnrol(t, keep.Token, "stale").Name != "stale" {
		t.Fatal("a revoked Host's name should be free again")
	}
}
