//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func signedIn(t *testing.T, prefix string) (*browser, string) {
	t.Helper()
	email := unique(prefix)
	return signInAs(t, email), email
}

func refresh(b *browser) int {
	b.t.Helper()
	resp := b.post("/auth/refresh")
	resp.Body.Close()
	return resp.StatusCode
}

func TestAccessTokenExpiresAfterFifteenMinutesAndRefreshRenewsIt(t *testing.T) {
	b, email := signedIn(t, "expiry")
	advance(t, 14*time.Minute)
	b.mustBeSignedIn(email)

	advance(t, 2*time.Minute)
	b.mustBeSignedOut()
	if code := refresh(b); code != http.StatusNoContent {
		t.Fatalf("refresh status %d", code)
	}
	b.mustBeSignedIn(email)
}

func TestRefreshRotatesAndKeepsSessionAlive(t *testing.T) {
	b, email := signedIn(t, "rotate")
	old := b.cookie("av_refresh").Value
	advance(t, 16*time.Minute)
	delete(b.jar(hostOf(platformURL)), "av_access")

	if code := refresh(b); code != http.StatusNoContent {
		t.Fatalf("refresh status %d", code)
	}
	if b.cookie("av_refresh").Value == old {
		t.Fatal("refresh token was not rotated")
	}
	b.mustBeSignedIn(email)
}

// TestSessions shares one User: every subtest signs in its own session, and a session's
// refresh chain is independent of the others.
func TestSessions(t *testing.T) {
	t.Parallel()
	email := unique("session")
	t.Run("sign-in creates User on first sign-in only", func(t *testing.T) { sessionsFirstSignIn(t, email) })
	t.Run("sign-in rule allows matching domain and group", func(t *testing.T) { signInAs(t, email) })
	t.Run("cookies are HttpOnly, Secure and Lax", func(t *testing.T) { sessionsCookies(t, email) })
	t.Run("concurrent refresh within the grace keeps the session", func(t *testing.T) { sessionsConcurrentRefresh(t, email) })
	t.Run("sign-out ends the session", func(t *testing.T) { sessionsSignOut(t, email) })
}

func sessionsConcurrentRefresh(t *testing.T, email string) {
	b := signInAs(t, email)
	tab := newBrowser(t)
	shared := *b.cookie("av_refresh")
	tab.jar(hostOf(platformURL))["av_refresh"] = &shared
	if code := refresh(b); code != http.StatusNoContent {
		t.Fatalf("first refresh %d", code)
	}
	if code := refresh(tab); code != http.StatusNoContent {
		t.Fatalf("second refresh within grace %d", code)
	}
	delete(b.jar(hostOf(platformURL)), "av_access")
	if code := refresh(b); code != http.StatusNoContent {
		t.Fatalf("chain revoked by concurrent refresh: %d", code)
	}
	b.mustBeSignedIn(email)
}

func TestRefreshTokenReuseAfterGraceRevokesTheChain(t *testing.T) {
	b, email := signedIn(t, "reuse")
	stolen := *b.cookie("av_refresh")
	if code := refresh(b); code != http.StatusNoContent {
		t.Fatalf("first refresh %d", code)
	}
	b.mustBeSignedIn(email)
	advance(t, time.Minute)

	thief := newBrowser(t)
	thief.jar(hostOf(platformURL))["av_refresh"] = &stolen
	if code := refresh(thief); code != http.StatusUnauthorized {
		t.Fatalf("reused token: status %d, want 401", code)
	}
	delete(b.jar(hostOf(platformURL)), "av_access")
	if code := refresh(b); code != http.StatusUnauthorized {
		t.Fatalf("chain survived reuse: %d", code)
	}
	b.mustBeSignedOut()
}

func TestRefreshTokenExpiresAfterSevenDays(t *testing.T) {
	b, _ := signedIn(t, "sevendays")
	advance(t, 7*24*time.Hour+time.Minute)
	if code := refresh(b); code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", code)
	}
}

func TestRefreshWithoutCookieIsUnauthorised(t *testing.T) {
	t.Parallel()
	if code := refresh(newBrowser(t)); code != http.StatusUnauthorized {
		t.Fatalf("status %d", code)
	}
}

func sessionsSignOut(t *testing.T, email string) {
	b := signInAs(t, email)
	refreshCookie := *b.cookie("av_refresh")

	resp := b.post("/auth/logout")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status %d", resp.StatusCode)
	}
	b.mustBeSignedOut()

	again := newBrowser(t)
	again.jar(hostOf(platformURL))["av_refresh"] = &refreshCookie
	if code := refresh(again); code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: %d", code)
	}
}

func TestTestClockEndpointOnlyExistsInTestBuilds(t *testing.T) {
	t.Parallel()
	resp, err := http.Post(prodURL+"/_test/clock", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("production build answered %d, want 404", resp.StatusCode)
	}
}
