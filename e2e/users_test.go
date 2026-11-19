//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func dataDir(userID string) string { return "/data/instances/" + userID }

func disable(b *browser, id string) int {
	return statusOf(b.post("/api/admin/users/" + id + "/disable"))
}

func TestDisabledUserIsRefusedEverywhere(t *testing.T) {
	t.Parallel()
	b, email, base := running(t, "disabled")
	e := mustEnrol(t, b.newToken().Token, "doomed")
	post := rawSyncUnknown(t, base)
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusOK {
		t.Fatalf("before disable: %d", code)
	}

	// Host authentication is upstream's: no Platform cookie, upstream's own 401 ("Unauthorized"; the Platform says "unauthorised").
	resp := post()
	status := resp.StatusCode
	raw := body(t, resp)
	if status != http.StatusUnauthorized || !strings.Contains(raw, "Unauthorized") {
		t.Fatalf("raw-sync: %d %s", status, raw)
	}

	sql(t, `update users set disabled = true where email = $1`, email)
	if code := statusOf(post()); code != http.StatusForbidden {
		t.Fatalf("Disabled User raw-sync: %d, want 403", code)
	}
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusForbidden {
		t.Fatalf("Disabled User's Host: %d, want 403", code)
	}

	if code := refresh(b); code != http.StatusUnauthorized {
		t.Fatalf("disabled User refreshed: %d", code)
	}
	delete(b.jar(hostOf(platformURL)), "av_access")
	b.mustBeSignedOut()

	b2 := newBrowser(t)
	b2.setIdentity(email, "Person", staff)
	if resp := b2.signIn(); resp.Request.URL.Path != "/not-permitted" {
		t.Fatalf("disabled User signed in: %s", resp.Request.URL)
	}
}

func TestUserLeavingTheRuleIsDisabledAndTheirInstanceStopped(t *testing.T) {
	t.Parallel()
	b, email, base := running(t, "rule-leaver")
	id := idOf(base)
	left := newBrowser(t)
	left.setIdentity(email, "Person", "other")
	if resp := left.signIn(); resp.Request.URL.Path != "/not-permitted" {
		t.Fatalf("got %s", resp.Request.URL)
	}
	if code := refresh(b); code != http.StatusUnauthorized {
		t.Fatalf("existing session survived being Disabled: %d", code)
	}

	admin := adminUser.browser(t)
	eventually(t, "Instance stopped", 20*time.Second, func() bool {
		return statusOf(admin.get("/i/"+id+"/api/ping")) == http.StatusServiceUnavailable
	})
	got := auditEntries(t, admin, "type=user_changes&q="+email)
	if len(got) != 1 || got[0]["action"] != "User disabled" {
		t.Fatalf("audit: %v", got)
	}
}

func TestDisablingAndReEnablingAUser(t *testing.T) {
	user, email, base := running(t, "leaving")
	id := idOf(base)
	e := mustEnrol(t, user.newToken().Token, "laptop")
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusOK {
		t.Fatalf("Host before disable: %d", code)
	}
	admin := adminUser.browser(t)

	if code := disable(admin, id); code != http.StatusNoContent {
		t.Fatalf("disable: %d", code)
	}
	if code := disable(admin, id); code != http.StatusNoContent {
		t.Fatalf("repeat disable: %d", code)
	}

	if code := refresh(user); code != http.StatusUnauthorized {
		t.Fatalf("refresh after disable: %d, want 401", code)
	}
	if n := count(t, `select count(*) from refresh_tokens where user_id = $1::uuid`, id); n != 0 {
		t.Fatalf("%d refresh tokens left", n)
	}
	admin.get("/api/instance?user=" + id).Body.Close()
	time.Sleep(3 * time.Second)
	if code := statusOf(admin.get("/i/" + id + "/api/ping")); code != http.StatusServiceUnavailable {
		t.Fatalf("Disabled User's Instance answers: %d, want 503", code)
	}

	advance(t, 16*time.Minute)
	user.mustBeSignedOut()
	refresh(admin)

	again := newBrowser(t)
	again.setIdentity(email, "Person", staff)
	if resp := again.signIn(); resp.Request.URL.Path != "/not-permitted" {
		t.Fatalf("Disabled User signed in: %s", resp.Request.URL)
	}

	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusForbidden {
		t.Fatalf("Host of Disabled User: %d, want 403", code)
	}
	if n := count(t, `select count(*) from hosts where user_id = $1::uuid and revoked_at is null`, id); n != 0 {
		t.Fatalf("%d Hosts not revoked", n)
	}
	if n := instanceCount(t, id, `select count(*) from agentsview.raw_devices where device_id = $1 and revoked_at is not null`, e.DeviceID); n != 1 {
		t.Fatal("device not revoked in the Instance")
	}

	if _, err := os.Stat(dataDir(id)); err != nil {
		t.Fatalf("data directory not kept: %v", err)
	}
	if n := count(t, `select count(*) from pg_database where datname = 'av_' || replace($1, '-', '')`, id); n != 1 {
		t.Fatal("Instance database not kept")
	}

	_, detail := call(admin, "GET", "/api/admin/users/"+id, nil)
	if detail["status"] != "disabled" {
		t.Fatalf("status %v", detail["status"])
	}
	got := auditEntries(t, admin, "type=user_changes&q="+email)
	if len(got) != 1 || got[0]["action"] != "User disabled" {
		t.Fatalf("audit: %v", got)
	}

	if code := impersonate(admin, id); code < 400 || code >= 500 {
		t.Errorf("impersonating a Disabled User: status %d, want 4xx", code)
	}
	if v := meAs(t, admin); v.ImpersonatedBy != nil {
		t.Fatal("a refused impersonation still started")
	}

	if code := statusOf(admin.post("/api/admin/users/" + id + "/enable")); code != http.StatusNoContent {
		t.Fatalf("enable: %d", code)
	}
	back := newBrowser(t)
	back.setIdentity(email, "Person", staff)
	back.signIn()
	back.mustBeSignedIn(email)
	back.waitInstance("running")
	got = auditEntries(t, admin, "type=user_changes&q="+email)
	if len(got) != 2 || got[0]["action"] != "User enabled" {
		t.Fatalf("audit: %v", got)
	}
}

func TestDeletingAUserRemovesTheirInstanceAndNeedsConfirmation(t *testing.T) {
	t.Parallel()
	user, email, base := running(t, "erased")
	id := idOf(base)
	e := mustEnrol(t, user.newToken().Token, "box")
	admin := adminUser.browser(t)
	path := "/api/admin/users/" + id + "/delete"

	for name, body := range map[string]map[string]string{"no confirmation": {}, "wrong confirmation": {"confirm": "someone@halcyon.example"}} {
		if resp, _ := call(admin, "POST", path, body); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: %d, want 400", name, resp.StatusCode)
		}
	}
	if n := count(t, `select count(*) from users where id = $1::uuid`, id); n != 1 {
		t.Fatal("User deleted without confirmation")
	}

	if resp, _ := call(admin, "POST", path, map[string]string{"confirm": email}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	for q, what := range map[string]string{
		`select count(*) from users where id = $1::uuid`:                                         "User row",
		`select count(*) from instances where user_id = $1::uuid`:                                "Instance row",
		`select count(*) from hosts where user_id = $1::uuid`:                                    "Host rows",
		`select count(*) from pg_database where datname = 'av_' || replace($1, '-', '')`:         "Instance database",
		`select count(*) from pg_roles where rolname like 'av\_' || replace($1, '-', '') || '%'`: "Instance roles",
	} {
		if n := count(t, q, id); n != 0 {
			t.Errorf("%s survived Delete", what)
		}
	}
	if _, err := os.Stat(dataDir(id)); !os.IsNotExist(err) {
		t.Errorf("data directory survived Delete: %v", err)
	}
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusNotFound {
		t.Errorf("deleted User's Host: %d, want 404", code)
	}
	user.mustBeSignedOut()
	got := auditEntries(t, admin, "type=user_changes&q="+email)
	if len(got) != 1 || got[0]["action"] != "User deleted" {
		t.Fatalf("audit: %v", got)
	}

	again := newBrowser(t)
	again.setIdentity(email, "Person", staff)
	again.signIn()
	again.mustBeSignedIn(email)
	if fresh := again.waitInstance("running"); fresh.URL == base {
		t.Fatal("new User reuses the deleted User's Instance")
	}
}

func adminDisableDeleteGuards(t *testing.T, _, subject *fx) {
	plain := plainUser.browser(t)
	admin := adminUser.browser(t)
	victimID := subject.id()
	if code := disable(plain, victimID); code != http.StatusForbidden {
		t.Fatalf("non-Admin disable: %d", code)
	}
	if resp, _ := call(plain, "POST", "/api/admin/users/"+victimID+"/delete", map[string]string{"confirm": subject.email}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-Admin delete: %d", resp.StatusCode)
	}
	if code := disable(admin, "00000000-0000-4000-8000-000000000000"); code != http.StatusNotFound {
		t.Fatalf("unknown User: %d", code)
	}
	self := userID(t, admin, adminUser.email)
	if code := disable(admin, self); code != http.StatusConflict {
		t.Fatalf("disabling yourself: %d, want 409", code)
	}
	if resp, _ := call(admin, "POST", "/api/admin/users/"+self+"/delete", map[string]string{"confirm": adminUser.email}); resp.StatusCode != http.StatusConflict {
		t.Fatalf("deleting yourself: %d, want 409", resp.StatusCode)
	}
}
