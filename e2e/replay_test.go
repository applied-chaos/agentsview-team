//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

type adminInstanceReply struct {
	ID    string `json:"id"`
	State string `json:"state"`
	URL   string `json:"url"`
}

func (b *browser) adminInstance() adminInstanceReply {
	b.t.Helper()
	return decode[adminInstanceReply](b.t, b.get("/api/admin/instance"))
}

// machineSessions counts the Admin Instance's sessions on one `<email>/<host>` device label.
func machineSessions(b *browser, base, label string) int {
	resp := b.get(base + "api/v1/sessions?limit=1&machine=" + url.QueryEscape(label))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return -1
	}
	return decode[struct {
		Total int `json:"total"`
	}](b.t, resp).Total
}

func testReplay(t *testing.T, body string) {
	t.Helper()
	testPost(t, "/_test/replay", body)
}

type replayRun struct {
	b               *browser
	email           string
	base, adminBase string
	host            *hostView
	label           string
	spoolBefore     map[string]bool
}

func (r *replayRun) with(t *testing.T) *replayRun {
	c := *r
	c.b = r.b.with(t)
	return &c
}

func TestReplay(t *testing.T) {
	t.Parallel()
	m := machineFor(t, "linux-profile")
	b, email := adminSignedIn(t, "replay")
	base := b.waitInstance("running").URL
	eventually(t, "Admin Instance running", 120*time.Second, func() bool { return b.adminInstance().State == "running" })
	adminBase := b.adminInstance().URL
	spoolBefore := spoolObjects(t)

	l := m.newLogin(t)
	l.writeSessions(2)
	tok := b.newToken()
	l.mustSh(installCommand(tok, "--profile"))
	_, host := b.tokenState(tok.ID)
	if host == nil {
		t.Fatal("no Host")
	}
	r := &replayRun{b: b, email: email, base: base, adminBase: adminBase, host: host, label: email + "/" + host.Name, spoolBefore: spoolBefore}

	for _, s := range []struct {
		name string
		fn   func(*testing.T, *replayRun)
	}{
		{"raw watch sessions reach the Instance and the Admin Instance as email/host", replayReaches},
		{"spool objects are deleted once the Admin Instance has committed them", replaySpoolPruned},
		{"only Admins open the Admin Instance", replayOnlyAdminsOpen},
		{"Admin Instance device is renamed after the Host", replayHostRenamed},
	} {
		t.Run(s.name, func(t *testing.T) { s.fn(t, r.with(t)) })
	}
}

func replayReaches(t *testing.T, r *replayRun) {
	b := r.b
	eventually(t, "sessions in the Instance", 180*time.Second, func() bool { return sessionCount(b, r.base) == 2 })
	eventually(t, "sessions in the Admin Instance as "+r.label, 120*time.Second, func() bool { return machineSessions(b, r.adminBase, r.label) == 2 })

	// A Host cannot reach the Admin Instance's raw-sync API through the Platform.
	resp := b.do("POST", platformURL+r.adminBase+"api/v1/raw-sync/tokens", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Admin Instance raw-sync from outside: %d, want 404", resp.StatusCode)
	}
}

func replayHostRenamed(t *testing.T, r *replayRun) {
	sql(t, `update hosts set name = 'renamed-'||name where id = $1`, r.host.ID)
	eventually(t, "device renamed after the Host", 60*time.Second, func() bool {
		return machineSessions(r.b, r.adminBase, r.email+"/renamed-"+r.host.Name) == 2
	})
}

func TestReplayResumesAfterARestartAndNeverDuplicates(t *testing.T) {
	m := machineFor(t, "linux-profile")
	b, email := adminSignedIn(t, "resume")
	base := b.waitInstance("running").URL
	l := m.newLogin(t)
	l.writeSessions(1)
	tok := b.newToken()
	l.mustSh(installCommand(tok, "--profile"))
	_, host := b.tokenState(tok.ID)
	eventually(t, "Admin Instance running", 120*time.Second, func() bool { return b.adminInstance().State == "running" })
	adminBase := b.adminInstance().URL
	label := email + "/" + host.Name
	eventually(t, "first session replayed", 180*time.Second, func() bool { return machineSessions(b, adminBase, label) == 1 })

	testReplay(t, `{"paused":true,"crashAfterCommit":true}`)
	l.writeSessionRange(1, 3)
	eventually(t, "new sessions in the Instance", 180*time.Second, func() bool { return sessionCount(b, base) == 3 })
	time.Sleep(5 * time.Second)
	if n := machineSessions(b, adminBase, label); n != 1 {
		t.Fatalf("Admin Instance has %d sessions while replay is stopped, want 1", n)
	}

	// The worker dies once after committing to the Admin Instance and before recording it.
	testReplay(t, `{"paused":false}`)
	eventually(t, "replay resumed", 120*time.Second, func() bool { return machineSessions(b, adminBase, label) == 3 })
	time.Sleep(5 * time.Second)
	if n := machineSessions(b, adminBase, label); n != 3 {
		t.Fatalf("Admin Instance has %d sessions after resume, want 3", n)
	}
}

func replayOnlyAdminsOpen(t *testing.T, r *replayRun) {
	admin := r.b
	info := admin.adminInstance()
	resp := admin.get(info.URL)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Admin opening the Admin Instance: %d", resp.StatusCode)
	}
	resp = admin.get("/api/admin/instance/open")
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != info.URL {
		t.Fatalf("open: %d %q, want redirect to %s", resp.StatusCode, resp.Header.Get("Location"), info.URL)
	}
	if n := len(auditEntries(t, admin, "type=instance_view")); n == 0 {
		t.Fatal("opening the Admin Instance was not audited")
	}

	user := plainUser.browser(t)
	for _, path := range []string{"/api/admin/instance", "/api/admin/instance/open", info.URL} {
		resp := user.get(path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("non-Admin GET %s: %d, want 403", path, resp.StatusCode)
		}
	}
	anon := newBrowser(t)
	if resp := anon.get("/api/admin/instance"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", resp.StatusCode)
	}
}

func TestDeletingAUserKeepsTheirSessionsInTheAdminInstance(t *testing.T) {
	m := machineFor(t, "linux-profile")
	user, email := signedIn(t, "kept")
	userBase := user.waitInstance("running").URL
	l := m.newLogin(t)
	l.writeSessions(2)
	tok := user.newToken()
	l.mustSh(installCommand(tok, "--profile"))
	_, host := user.tokenState(tok.ID)
	admin := adminUser.browser(t)
	eventually(t, "Admin Instance running", 120*time.Second, func() bool { return admin.adminInstance().State == "running" })
	adminBase := admin.adminInstance().URL
	label := email + "/" + host.Name
	eventually(t, "sessions in the Admin Instance", 180*time.Second, func() bool { return machineSessions(admin, adminBase, label) == 2 })

	// Sessions the worker has not replayed yet must also survive the Delete.
	testReplay(t, `{"paused":true}`)
	t.Cleanup(func() { testReplay(t, `{"paused":false}`) })
	l.writeSessionRange(2, 5)
	eventually(t, "new sessions in the Instance", 180*time.Second, func() bool { return sessionCount(user, userBase) == 5 })

	resp, _ := call(admin, "POST", "/api/admin/users/"+idOf(userBase)+"/delete", map[string]string{"confirm": email})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	eventually(t, "all the deleted User's sessions in the Admin Instance", 60*time.Second, func() bool { return machineSessions(admin, adminBase, label) == 5 })
}
