//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"testing"
)

// onlyAdmin makes email the single Admin; the database is shared across tests.
func onlyAdmin(t *testing.T, email string) {
	t.Helper()
	sql(t, `update users set is_admin = (email = $1)`, email)
	sql(t, `delete from pending_admins`)
}

func cli(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("platform", append([]string{"admin"}, args...)...)
	cmd.Env = []string{"PLATFORM_DATABASE_URL=" + databaseURL}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func call(b *browser, method, path string, body any) (*http.Response, map[string]any) {
	b.t.Helper()
	var resp *http.Response
	if body != nil {
		raw, _ := json.Marshal(body)
		resp = b.do(method, platformURL+path, bytes.NewReader(raw))
	} else {
		resp = b.do(method, platformURL+path, nil)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func adminSignedIn(t *testing.T, prefix string) (*browser, string) {
	t.Helper()
	b, email := signedIn(t, prefix)
	sql(t, `update users set is_admin = true where email = $1`, email)
	return b, email
}

func userID(t *testing.T, b *browser, email string) string {
	t.Helper()
	_, list := call(b, "GET", "/api/admin/users", nil)
	for _, u := range list["users"].([]any) {
		if u := u.(map[string]any); u["email"] == email {
			id, _ := u["id"].(string)
			return id
		}
	}
	t.Fatalf("%s not in Users list", email)
	return ""
}

func auditEntries(t *testing.T, b *browser, query string) []map[string]any {
	t.Helper()
	resp, out := call(b, "GET", "/api/admin/audit?range=all&"+query, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit status %d", resp.StatusCode)
	}
	var rows []map[string]any
	for _, e := range out["entries"].([]any) {
		rows = append(rows, e.(map[string]any))
	}
	return rows
}

// target is impersonated and viewed; subject has Admin granted and revoked, and owns the Hosts and
// Install Tokens on the detail page.
func TestAdmin(t *testing.T) {
	t.Parallel()
	target, subject := newFx(t, "target"), newFx(t, "subject")
	name := rename(t, target.email)
	run := func(label string, fn func(*testing.T, *fx, *fx, string)) {
		t.Run(label, func(t *testing.T) { fn(t, target.with(t), subject.with(t), name) })
	}
	plain := func(fn func(*testing.T, *fx, *fx)) func(*testing.T, *fx, *fx, string) {
		return func(t *testing.T, a, b *fx, _ string) { fn(t, a, b) }
	}

	run("APIs are Admin only", plain(adminAPIsAdminOnly))
	run("grant, revoke and audit", plain(adminGrantRevokeAudit))
	run("CLI grant stays pending until first sign-in", plain(adminCLIGrantPending))
	run("CLI revoke drops pending grant and removes Admin", plain(adminCLIRevoke))
	run("revoked Admin loses Admin APIs at once", plain(adminRevokedLosesAPIs))
	run("audit log records CLI actions and filters by category", plain(adminAuditCLI))
	run("Admin user detail lists Hosts and Install Tokens", plain(adminUserDetail))
	run("disable and delete are Admin only and never self", plain(adminDisableDeleteGuards))
	run("cross-site state changes are refused", plain(adminCrossSite))
	run("Instance View is full access and not audited", impersonationViewNotAudited)
	run("Instance View and impersonation are Admin only", impersonationAdminOnly)
	run("impersonation acts as the User, ends and is audited", impersonationActsAs)
	run("impersonation refuses bad targets", impersonationBadTargets)
	run("signing out ends impersonation and audits it", impersonationSignOut)
	run("revoked Admin loses impersonation at once", impersonationRevokedAdmin)
	run("everything done while impersonating is audited", impersonationEverythingAudited)
}

func adminAPIsAdminOnly(t *testing.T, _, _ *fx) {
	anon := newBrowser(t)
	user := plainUser.browser(t)
	body := map[string]string{"email": "x@halcyon.example"}
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/admin/users"}, {"GET", "/api/admin/audit"}, {"GET", "/api/admin/instance"},
		{"POST", "/api/admin/admins"}, {"GET", "/api/admin/users/00000000-0000-0000-0000-000000000000"},
	} {
		if resp, _ := call(anon, c.method, c.path, body); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("signed out %s %s: %d", c.method, c.path, resp.StatusCode)
		}
		if resp, _ := call(user, c.method, c.path, body); resp.StatusCode != http.StatusForbidden {
			t.Errorf("User %s %s: %d", c.method, c.path, resp.StatusCode)
		}
	}
}

func demoteLater(t *testing.T, email string) {
	t.Helper()
	t.Cleanup(func() { sql(t, `update users set is_admin = false where email = $1`, email) })
}

func adminGrantRevokeAudit(t *testing.T, _, subject *fx) {
	b := adminUser.browser(t)
	target, targetEmail := subject.b, subject.email
	demoteLater(t, targetEmail)

	resp, out := call(b, "POST", "/api/admin/admins", map[string]string{"email": targetEmail})
	if resp.StatusCode != http.StatusOK || out["pending"] != false {
		t.Fatalf("grant existing: %d %v", resp.StatusCode, out)
	}
	if m := target.mustBeSignedIn(targetEmail); !m.IsAdmin {
		t.Fatal("not Admin after grant")
	}

	pendingEmail := unique("later")
	resp, out = call(b, "POST", "/api/admin/admins", map[string]string{"email": pendingEmail})
	if resp.StatusCode != http.StatusOK || out["pending"] != true {
		t.Fatalf("grant unknown: %d %v", resp.StatusCode, out)
	}
	if resp, _ := call(b, "POST", "/api/admin/admins", map[string]string{"email": "not an email"}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad email: %d", resp.StatusCode)
	}

	_, list := call(b, "GET", "/api/admin/users", nil)
	var sawPending bool
	for _, u := range list["users"].([]any) {
		if u := u.(map[string]any); u["email"] == pendingEmail && u["pendingAdmin"] == true {
			sawPending = true
		}
	}
	if !sawPending {
		t.Fatal("pending grant missing from Users list")
	}

	id := userID(t, b, targetEmail)
	if resp, _ := call(b, "PUT", "/api/admin/users/"+id+"/role", map[string]bool{"admin": false}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: %d", resp.StatusCode)
	}
	if m := target.mustBeSignedIn(targetEmail); m.IsAdmin {
		t.Fatal("still Admin after revoke")
	}

	_, detail := call(b, "GET", "/api/admin/users/"+id, nil)
	if detail["email"] != targetEmail || detail["role"] != "user" {
		t.Fatalf("detail %v", detail)
	}

	var granted, removed bool
	for _, e := range auditEntries(t, b, "q="+targetEmail) {
		granted = granted || e["action"] == "Admin granted" && e["actor"] == "Person"
		removed = removed || e["action"] == "Admin role removed" && e["actor"] == "Person"
	}
	if !granted || !removed {
		t.Fatalf("audit missing grant=%v remove=%v", granted, removed)
	}
	if n := len(auditEntries(t, b, "q="+pendingEmail)); n != 1 {
		t.Fatalf("pending grant audit entries: %d", n)
	}
}

func adminCLIGrantPending(t *testing.T, _, _ *fx) {
	email := unique("owen")
	if out, err := cli(t, "grant", email); err != nil {
		t.Fatalf("grant: %v %s", err, out)
	}
	if n := count(t, `select count(*) from users where email = $1`, email); n != 0 {
		t.Fatal("grant must not create the User")
	}

	b := newBrowser(t)
	b.setIdentity(email, "Owen", staff)
	b.signIn()
	if m := b.mustBeSignedIn(email); !m.IsAdmin {
		t.Fatal("pending grant was not applied at first sign-in")
	}
	if n := count(t, `select count(*) from pending_admins where email = $1`, email); n != 0 {
		t.Fatal("pending grant not consumed")
	}
}

func adminCLIRevoke(t *testing.T, _, _ *fx) {
	email := unique("pat")
	if out, err := cli(t, "grant", email); err != nil {
		t.Fatal(err, out)
	}
	if out, err := cli(t, "revoke", email); err != nil {
		t.Fatalf("revoke pending: %v %s", err, out)
	}
	b := newBrowser(t)
	b.setIdentity(email, "Pat", staff)
	b.signIn()
	if m := b.mustBeSignedIn(email); m.IsAdmin {
		t.Fatal("revoked pending grant still applied")
	}

	if out, err := cli(t, "grant", email); err != nil {
		t.Fatalf("grant existing: %v %s", err, out)
	}
	if m := b.mustBeSignedIn(email); !m.IsAdmin {
		t.Fatal("grant to an existing User is immediate")
	}
	if out, err := cli(t, "revoke", email); err != nil {
		t.Fatalf("revoke: %v %s", err, out)
	}
	if m := b.mustBeSignedIn(email); m.IsAdmin {
		t.Fatal("revoke did not remove Admin")
	}
}

func adminRevokedLosesAPIs(t *testing.T, _, subject *fx) {
	demoteLater(t, subject.email)
	sql(t, `update users set is_admin = true where email = $1`, subject.email)
	b := signInAs(t, subject.email)
	if resp, _ := call(b, "GET", "/api/admin/users", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin list: %d", resp.StatusCode)
	}
	sql(t, `update users set is_admin = false where email = $1`, subject.email)
	if resp, _ := call(b, "GET", "/api/admin/users", nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("after revoke: %d", resp.StatusCode)
	}
}

func adminAuditCLI(t *testing.T, _, _ *fx) {
	b := adminUser.browser(t)
	email := unique("cli-target")
	if out, err := cli(t, "grant", email); err != nil {
		t.Fatal(err, out)
	}
	var found bool
	for _, e := range auditEntries(t, b, "q="+email) {
		found = found || e["action"] == "Admin granted" && e["actor"] == "CLI"
	}
	if !found {
		t.Fatal("CLI grant not audited as CLI")
	}
	if n := len(auditEntries(t, b, "type=hosts&q="+email)); n != 0 {
		t.Fatalf("hosts filter should hide admin grants, got %d", n)
	}
	if n := len(auditEntries(t, b, "type=admin_grants&q="+email)); n != 1 {
		t.Fatalf("admin_grants filter: %d", n)
	}
}

// Serial: the Admin set is Platform-wide. The CLI removal of the last Admin runs last.
func TestLastAdminGuardAndInstanceUpgradeFailure(t *testing.T) {
	admin := adminUser.browser(t)
	onlyAdmin(t, adminUser.email)

	t.Run("last Admin cannot be removed in the UI", func(t *testing.T) {
		admin := admin.with(t)
		id := userID(t, admin, adminUser.email)
		resp, out := call(admin, "PUT", "/api/admin/users/"+id+"/role", map[string]bool{"admin": false})
		if resp.StatusCode != http.StatusConflict || out["error"] != "last_admin" {
			t.Fatalf("want 409 last_admin, got %d %v", resp.StatusCode, out)
		}
		if count(t, `select count(*) from users where email = $1 and is_admin`, adminUser.email) != 1 {
			t.Fatal("last Admin was removed")
		}
	})

	t.Run("an Instance whose upgrade fails is marked failed while others keep running", func(t *testing.T) {
		upgradeFailure(t, admin.with(t))
	})

	t.Run("CLI can remove the last Admin", func(t *testing.T) {
		if out, err := cli(t, "revoke", adminUser.email); err != nil {
			t.Fatalf("CLI revoke of the last Admin: %v %s", err, out)
		}
		if count(t, `select count(*) from users where email = $1 and is_admin`, adminUser.email) != 0 {
			t.Fatal("last Admin was not removed")
		}
	})
}
