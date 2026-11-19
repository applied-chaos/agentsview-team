//go:build e2e

// Package e2e drives the Platform over HTTP against the compose stack
// (real Postgres, fake OIDC issuer). Run with `make e2e`.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	platformURL = env("PLATFORM_URL", "http://platform:8080")
	prodURL     = env("PLATFORM_PROD_URL", "http://platform-prod:8080")
	issuerURL   = env("ISSUER_URL", "http://fakeoidc:9000")
	databaseURL = env("DATABASE_URL", "postgres://postgres:postgres@postgres:5432/platform")
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// browser is a cookie-keeping HTTP client. net/http/cookiejar refuses to send
// Secure cookies over plain http, which the test stack uses, so cookies are
// tracked by hand; that also lets tests assert on their attributes.
type browser struct {
	t       *testing.T
	cookies map[string]map[string]*http.Cookie // host -> name -> cookie
	client  *http.Client
	headers map[string]string // sent with every request, e.g. Origin
}

func newBrowser(t *testing.T) *browser {
	return &browser{
		t:       t,
		cookies: map[string]map[string]*http.Cookie{},
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			Timeout:       15 * time.Second,
		},
	}
}

func (b *browser) setIdentity(email, name string, groups ...string) {
	b.setIdentityVerified(email, name, "true", groups...)
}

// setIdentityVerified sets the email_verified claim: "true", "false" or "omit".
func (b *browser) setIdentityVerified(email, name, verified string, groups ...string) {
	u, _ := url.Parse(issuerURL)
	v := url.QueryEscape(email + "|" + name + "|" + strings.Join(groups, ",") + "|" + verified)
	b.jar(u.Host)["fake_identity"] = &http.Cookie{Name: "fake_identity", Value: v}
}

func (b *browser) jar(host string) map[string]*http.Cookie {
	if b.cookies[host] == nil {
		b.cookies[host] = map[string]*http.Cookie{}
	}
	return b.cookies[host]
}

func (b *browser) cookie(name string) *http.Cookie {
	u, _ := url.Parse(platformURL)
	return b.jar(u.Host)[name]
}

func (b *browser) do(method, rawurl string, body io.Reader) *http.Response {
	b.t.Helper()
	req, err := http.NewRequest(method, rawurl, body)
	if err != nil {
		b.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range b.headers {
		req.Header.Set(k, v)
	}
	jar := b.jar(req.URL.Host)
	for _, c := range jar {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(jar, c.Name)
		} else {
			jar[c.Name] = c
		}
	}
	return resp
}

func (b *browser) get(path string) *http.Response  { return b.do("GET", platformURL+path, nil) }
func (b *browser) post(path string) *http.Response { return b.do("POST", platformURL+path, nil) }

func (b *browser) follow(resp *http.Response) *http.Response {
	b.t.Helper()
	for i := 0; i < 10; i++ {
		loc := resp.Header.Get("Location")
		if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
			return resp
		}
		resp.Body.Close()
		base, _ := url.Parse(resp.Request.URL.String())
		next, err := base.Parse(loc)
		if err != nil {
			b.t.Fatal(err)
		}
		resp = b.do("GET", next.String(), nil)
	}
	b.t.Fatal("too many redirects")
	return nil
}

func (b *browser) signIn() *http.Response {
	b.t.Helper()
	return b.follow(b.get("/auth/login"))
}

type me struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"isAdmin"`
}

func (b *browser) me() (me, int) {
	b.t.Helper()
	resp := b.get("/api/me")
	defer resp.Body.Close()
	var m me
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			b.t.Fatal(err)
		}
	}
	return m, resp.StatusCode
}

func (b *browser) mustBeSignedIn(email string) me {
	b.t.Helper()
	m, code := b.me()
	if code != http.StatusOK || m.Email != email {
		b.t.Fatalf("want signed in as %s, got status %d, %+v", email, code, m)
	}
	return m
}

func (b *browser) mustBeSignedOut() {
	b.t.Helper()
	if _, code := b.me(); code != http.StatusUnauthorized {
		b.t.Fatalf("want 401, got %d", code)
	}
}

// clockOffset is how far the tests have moved the Platform's clock, so test data can be dated
// inside the Platform's idea of the last 30 days.
var clockOffset time.Duration

func advance(t *testing.T, d time.Duration) {
	t.Helper()
	resp, err := http.Post(platformURL+"/_test/clock", "application/json",
		strings.NewReader(fmt.Sprintf(`{"advance":%q}`, d.String())))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("clock advance: status %d", resp.StatusCode)
	}
	clockOffset += d
}

func refreshStats(t *testing.T) {
	t.Helper()
	resp, err := http.Post(platformURL+"/_test/stats/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("stats refresh: status %d", resp.StatusCode)
	}
}

func sql(t *testing.T, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var n int
	if err := conn.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func unique(prefix string) string {
	return fmt.Sprintf("%s-%d@halcyon.example", prefix, time.Now().UnixNano())
}

func hostOf(raw string) string {
	u, _ := url.Parse(raw)
	return u.Host
}
