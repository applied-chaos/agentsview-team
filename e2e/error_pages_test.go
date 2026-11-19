//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func htmlBrowser(t *testing.T) *browser {
	b := newBrowser(t)
	b.headers = map[string]string{"Accept": "text/html,application/xhtml+xml"}
	return b
}

func (b *browser) setFault(email, fault string) {
	u, _ := url.Parse(issuerURL)
	v := url.QueryEscape(email + "|Person|" + staff + "|true|" + fault)
	b.jar(u.Host)["fake_identity"] = &http.Cookie{Name: "fake_identity", Value: v}
}

func wantErrorPage(t *testing.T, resp *http.Response, status int, reason string) {
	t.Helper()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d (%s)", resp.StatusCode, status, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type %q, want HTML (%s)", ct, body)
	}
	if want := `<meta name="av-error" content="` + reason + `">`; !strings.Contains(string(body), want) {
		t.Fatalf("body lacks %s: %s", want, body)
	}
}

func loginState(t *testing.T, b *browser) string {
	t.Helper()
	resp := b.get("/auth/login")
	resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Query().Get("state") == "" {
		t.Fatalf("no state in login redirect %q", resp.Header.Get("Location"))
	}
	return loc.Query().Get("state")
}

func TestCallbackFailuresServeStyledPages(t *testing.T) {
	t.Parallel()
	t.Run("no sign-in in progress", func(t *testing.T) {
		wantErrorPage(t, htmlBrowser(t).get("/auth/callback"), http.StatusBadRequest, "sign-in-expired")
	})
	t.Run("state mismatch", func(t *testing.T) {
		b := htmlBrowser(t)
		loginState(t, b)
		wantErrorPage(t, b.get("/auth/callback?state=wrong&code=x"), http.StatusBadRequest, "sign-in-expired")
	})
	t.Run("provider refused", func(t *testing.T) {
		b := htmlBrowser(t)
		s := loginState(t, b)
		wantErrorPage(t, b.get("/auth/callback?error=access_denied&state="+s), http.StatusBadRequest, "provider-refused")
	})
	t.Run("code exchange fails", func(t *testing.T) {
		b := htmlBrowser(t)
		s := loginState(t, b)
		wantErrorPage(t, b.get("/auth/callback?code=bogus&state="+s), http.StatusBadGateway, "provider-unavailable")
	})
	for _, fault := range []string{"nonce", "hs256bad"} {
		t.Run("bad token "+fault, func(t *testing.T) {
			b := htmlBrowser(t)
			b.setFault(unique("fault"), fault)
			wantErrorPage(t, b.signIn(), http.StatusBadRequest, "bad-identity")
			b.mustBeSignedOut()
		})
	}
	t.Run("unverified email", func(t *testing.T) {
		b := htmlBrowser(t)
		b.setIdentityVerified(unique("unverified"), "Person", "false", staff)
		wantErrorPage(t, b.signIn(), http.StatusBadRequest, "no-verified-email")
	})
}

// HS256 is signed with the client secret; RS512 is not in the issuer's discovery document.
func TestSignInAcceptsHS256AndUnlistedAlgorithms(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"hs256", "rs512"} {
		email := unique(fault)
		b := newBrowser(t)
		b.setFault(email, fault)
		b.signIn()
		b.mustBeSignedIn(email)
	}
}

func TestOtherBrowserErrorsServeStyledPages(t *testing.T) {
	t.Parallel()
	b := htmlBrowser(t)
	wantErrorPage(t, b.get("/i/not-an-instance/"), http.StatusNotFound, "not-found")
}

func TestErrorsStayPlainForNonBrowsers(t *testing.T) {
	t.Parallel()
	b := newBrowser(t)
	for _, path := range []string{"/auth/callback", "/api/nope"} {
		resp := b.get(path)
		resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s without Accept: HTML %q", path, ct)
		}
	}
	h := htmlBrowser(t)
	resp := h.get("/api/nope")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("/api/nope with Accept html: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}
