//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type viewer struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	IsAdmin        bool   `json:"isAdmin"`
	ImpersonatedBy *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"impersonatedBy"`
}

func meAs(t *testing.T, b *browser) viewer {
	t.Helper()
	resp := b.get("/api/me")
	defer resp.Body.Close()
	var v viewer
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func impersonate(b *browser, id string) int {
	b.t.Helper()
	return statusOf(b.post("/auth/impersonate/" + id))
}

func stopImpersonating(b *browser) int {
	b.t.Helper()
	return statusOf(b.post("/auth/impersonate/stop"))
}

// rename gives a User a unique name so audit searches find only their entries.
func rename(t *testing.T, email string) string {
	t.Helper()
	name := "Named-" + strings.Split(email, "@")[0]
	sql(t, `update users set name = $1 where email = $2`, name, email)
	return name
}

func idOf(base string) string { return strings.Split(strings.Trim(base, "/"), "/")[1] }

func impersonationViewNotAudited(t *testing.T, target, _ *fx, name string) {
	base := target.base
	admin := adminUser.browser(t)
	before := len(auditEntries(t, admin, "q="+name))

	page := admin.get(base)
	html := body(t, page)
	if page.StatusCode != http.StatusOK || !strings.Contains(html, `id="av-platbar"`) {
		t.Fatalf("Instance View page: %d %.200s", page.StatusCode, html)
	}
	if !strings.Contains(html, `class="banner view"`) || !strings.Contains(html, "Instance View: you are looking at "+name) ||
		!strings.Contains(html, "Exit Instance View") {
		t.Fatalf("no Instance View banner: %.800s", html)
	}
	if code := statusOf(admin.get(base + "api/v1/stats")); code != http.StatusOK {
		t.Fatalf("stats in Instance View: %d", code)
	}
	if n := len(auditSince(t, admin, "q="+name, before)); n != 0 {
		t.Fatalf("Instance View was audited (%d entries)", n)
	}
}

func impersonationAdminOnly(t *testing.T, target, _ *fx, _ string) {
	base := target.base
	plain := plainUser.browser(t)

	if code := statusOf(plain.get(base)); code != http.StatusForbidden {
		t.Fatalf("non-Admin Instance View: %d, want 403", code)
	}
	if code := impersonate(plain, idOf(base)); code != http.StatusForbidden {
		t.Fatalf("non-Admin impersonate: %d, want 403", code)
	}
	if v := meAs(t, plain); v.ImpersonatedBy != nil {
		t.Fatal("non-Admin ended up impersonating")
	}
	if code := impersonate(newBrowser(t), idOf(base)); code != http.StatusUnauthorized {
		t.Fatalf("signed-out impersonate: %d, want 401", code)
	}
}

func impersonationActsAs(t *testing.T, target, _ *fx, name string) {
	base, targetEmail := target.base, target.email
	_, otherBase := plainUser.instance(t)
	admin := adminUser.browser(t)
	adminEmail := adminUser.email
	before := len(auditEntries(t, admin, "type=impersonation&q="+name))

	if code := impersonate(admin, idOf(base)); code != http.StatusNoContent {
		t.Fatalf("impersonate: %d", code)
	}
	v := meAs(t, admin)
	if v.Email != targetEmail || v.Name != name || v.ImpersonatedBy == nil || v.ImpersonatedBy.Email != adminEmail || v.IsAdmin {
		t.Fatalf("/api/me while impersonating: %+v", v)
	}

	if resp := admin.get("/i/me/"); resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), base) {
		t.Fatalf("/i/me/ while impersonating: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	html := body(t, admin.get(base))
	if !strings.Contains(html, `class="banner imp"`) || !strings.Contains(html, "Impersonating "+name+".") ||
		!strings.Contains(html, "Stop impersonating") {
		t.Fatalf("no impersonation banner: %.800s", html)
	}
	if code := statusOf(admin.get(otherBase)); code != http.StatusForbidden {
		t.Fatalf("other Instance while impersonating: %d, want 403", code)
	}
	if code := statusOf(admin.get("/api/admin/users")); code != http.StatusForbidden {
		t.Fatalf("Admin API while impersonating: %d, want 403", code)
	}
	if code := impersonate(admin, idOf(base)); code != http.StatusConflict {
		t.Fatalf("nested impersonate: %d, want 409", code)
	}

	if refresh(admin) != http.StatusNoContent {
		t.Fatal("refresh failed")
	}
	if v := meAs(t, admin); v.Email != targetEmail || v.ImpersonatedBy == nil {
		t.Fatalf("impersonation lost on refresh: %+v", v)
	}

	if code := stopImpersonating(admin); code != http.StatusNoContent {
		t.Fatalf("stop: %d", code)
	}
	if v := meAs(t, admin); v.Email != adminEmail || v.ImpersonatedBy != nil || !v.IsAdmin {
		t.Fatalf("/api/me after stop: %+v", v)
	}
	if refresh(admin) != http.StatusNoContent {
		t.Fatal("refresh after stop failed")
	}
	if v := meAs(t, admin); v.ImpersonatedBy != nil {
		t.Fatal("impersonation came back after refresh")
	}
	if code := stopImpersonating(admin); code != http.StatusNoContent {
		t.Fatalf("second stop: %d", code)
	}

	entries := auditSince(t, admin, "type=impersonation&q="+name, before)
	if len(entries) != 2 {
		t.Fatalf("want start and end entries, got %v", entries)
	}
	if entries[0]["action"] != "Impersonation ended" || entries[1]["action"] != "Impersonation started" {
		t.Fatalf("audit actions (newest first): %v", entries)
	}
	for _, e := range entries {
		if e["actor"] != "Person" || e["target"] != name {
			t.Fatalf("audit names: %v", e)
		}
	}
	if d, _ := entries[0]["details"].(string); !strings.HasPrefix(d, "Lasted ") {
		t.Fatalf("end details %q", d)
	}
}

func impersonationBadTargets(t *testing.T, _, _ *fx, _ string) {
	admin := adminUser.browser(t)
	for name, id := range map[string]string{
		"self":    meAs(t, admin).ID,
		"unknown": "00000000-0000-4000-8000-000000000000",
		"garbage": "not-a-uuid",
	} {
		if code := impersonate(admin, id); code < 400 || code >= 500 {
			t.Errorf("%s: status %d, want 4xx", name, code)
		}
	}
	if v := meAs(t, admin); v.ImpersonatedBy != nil {
		t.Fatal("a refused impersonation still started")
	}
}

func impersonationSignOut(t *testing.T, target, _ *fx, name string) {
	admin := adminUser.browser(t)
	before := len(auditEntries(t, admin, "type=impersonation&q="+name))
	if impersonate(admin, idOf(target.base)) != http.StatusNoContent {
		t.Fatal("impersonate failed")
	}
	if code := statusOf(admin.post("/auth/logout")); code != http.StatusNoContent {
		t.Fatalf("logout: %d", code)
	}
	reader := adminUser.browser(t)
	got := auditSince(t, reader, "type=impersonation&q="+name, before)
	if len(got) != 2 || got[0]["action"] != "Impersonation ended" {
		t.Fatalf("sign-out did not audit the end: %v", got)
	}
}

func impersonationRevokedAdmin(t *testing.T, target, subject *fx, _ string) {
	demoteLater(t, subject.email)
	sql(t, `update users set is_admin = true where email = $1`, subject.email)
	admin := signInAs(t, subject.email)
	if impersonate(admin, idOf(target.base)) != http.StatusNoContent {
		t.Fatal("impersonate failed")
	}
	sql(t, `update users set is_admin = false where email = $1`, subject.email)
	if code := statusOf(admin.get(target.base)); code != http.StatusForbidden {
		t.Fatalf("revoked Admin still impersonating: %d", code)
	}
}
