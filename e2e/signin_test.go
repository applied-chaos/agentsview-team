//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

const staff = "staff"

func sessionsFirstSignIn(t *testing.T, email string) {
	b := newBrowser(t)
	b.setIdentity(email, "Alice Example", staff)

	resp := b.signIn()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/" {
		t.Fatalf("want landing on /, got %d %s", resp.StatusCode, resp.Request.URL)
	}
	first := b.mustBeSignedIn(email)
	if first.Name != "Alice Example" || first.IsAdmin {
		t.Fatalf("unexpected profile %+v", first)
	}

	b2 := newBrowser(t)
	b2.setIdentity(email, "Alice Example", staff)
	b2.signIn()
	if second := b2.mustBeSignedIn(email); second.ID != first.ID {
		t.Fatalf("second sign-in made a new User: %s vs %s", second.ID, first.ID)
	}
}

func sessionsCookies(t *testing.T, email string) {
	b := newBrowser(t)
	b.setIdentity(email, "C", staff)
	b.signIn()
	for _, name := range []string{"av_access", "av_refresh"} {
		c := b.cookie(name)
		if c == nil {
			t.Fatalf("no %s cookie", name)
		}
		if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
			t.Errorf("%s: HttpOnly=%v Secure=%v SameSite=%v", name, c.HttpOnly, c.Secure, c.SameSite)
		}
	}
	if got := b.cookie("av_access").MaxAge; got != 15*60 {
		t.Errorf("access cookie max-age %d, want 900", got)
	}
	if got := b.cookie("av_refresh").MaxAge; got != 7*24*3600 {
		t.Errorf("refresh cookie max-age %d, want 604800", got)
	}
}

func TestSignInRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		email  string
		groups []string
	}{
		{"wrong domain", unique("x") + ".gmail.example", []string{staff}},
		{"right domain, missing group", unique("nogroup"), []string{"other"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newBrowser(t)
			email := c.email
			b.setIdentity(email, "Person", c.groups...)
			resp := b.signIn()
			if resp.Request.URL.Path != "/not-permitted" {
				t.Fatalf("want /not-permitted, got %s", resp.Request.URL)
			}
			b.mustBeSignedOut()
			if b.cookie("av_refresh") != nil {
				t.Fatal("refresh cookie issued to a non-matching person")
			}
			if n := count(t, "select count(*) from users where email = $1", email); n != 0 {
				t.Fatalf("User created for non-matching person")
			}
			dr := b.get("/api/denied")
			defer dr.Body.Close()
			var denied struct {
				Email string `json:"email"`
			}
			if err := json.NewDecoder(dr.Body).Decode(&denied); err != nil || denied.Email != email {
				t.Fatalf("denied payload %+v err %v", denied, err)
			}
		})
	}
}

func TestSignInNeedsAVerifiedEmail(t *testing.T) {
	t.Parallel()
	for _, verified := range []string{"false", "omit"} {
		email := unique("unverified")
		b := newBrowser(t)
		b.setIdentityVerified(email, "Person", verified, staff)
		if resp := b.signIn(); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("email_verified %s: %d, want 400", verified, resp.StatusCode)
		}
		b.mustBeSignedOut()
		if n := count(t, "select count(*) from users where email = $1", email); n != 0 {
			t.Fatalf("User created for an unverified email (%s)", verified)
		}
	}
}

func TestPublicConfigDescribesTheRule(t *testing.T) {
	t.Parallel()
	resp := newBrowser(t).get("/api/public")
	defer resp.Body.Close()
	var p struct {
		ProviderName string   `json:"providerName"`
		Domains      []string `json:"domains"`
		Group        string   `json:"group"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.ProviderName != "Halcyon" || len(p.Domains) != 1 || p.Domains[0] != "halcyon.example" || p.Group != staff {
		t.Fatalf("unexpected %+v", p)
	}
}

func TestSPAIsServedForClientRoutes(t *testing.T) {
	t.Parallel()
	b := newBrowser(t)
	for _, path := range []string{"/", "/sign-in", "/not-permitted", "/hosts"} {
		resp := b.get(path)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Errorf("%s: %d %s", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		resp.Body.Close()
	}
	resp := b.get("/api/nope")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown API route: %d, want 404", resp.StatusCode)
	}
}

func TestLoginCallbackRejectsForgedState(t *testing.T) {
	t.Parallel()
	b := newBrowser(t)
	b.setIdentity(unique("forge"), "F", staff)
	b.get("/auth/login").Body.Close()
	resp := b.get("/auth/callback?" + url.Values{"code": {"x"}, "state": {"forged"}}.Encode())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
	b.mustBeSignedOut()
}
