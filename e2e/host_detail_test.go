//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func (b *browser) send(method, path, payload string) *http.Response {
	b.t.Helper()
	return b.do(method, platformURL+path, strings.NewReader(payload))
}

func (b *browser) hostByID(id string) (hostView, int) {
	b.t.Helper()
	resp := b.get("/api/hosts/" + id)
	if resp.StatusCode != http.StatusOK {
		return hostView{}, statusOf(resp)
	}
	return decode[struct {
		Host hostView `json:"host"`
	}](b.t, resp).Host, http.StatusOK
}

func enrolled(t *testing.T, b *browser, name string) (hostView, enrolReply) {
	t.Helper()
	e := mustEnrol(t, b.newToken().Token, name)
	h := b.findHost(e.Name)
	if h == nil {
		t.Fatalf("Host %q missing after enrol", name)
	}
	return *h, e
}

func hostsRename(t *testing.T, f *fx) {
	b := f.b
	me, _ := b.me()
	a, ea := enrolled(t, b, "rename-alpha")
	other, _ := enrolled(t, b, "rename-beta")

	if code := statusOf(b.send("PATCH", "/api/hosts/"+a.ID, `{"name":"rename-workstation"}`)); code != http.StatusNoContent {
		t.Fatalf("rename: %d", code)
	}
	if h, _ := b.hostByID(a.ID); h.Name != "rename-workstation" {
		t.Fatalf("detail name %q", h.Name)
	}
	if n := instanceCount(t, me.ID, `select count(*) from agentsview.raw_devices where device_id = $1 and display_name = 'rename-workstation'`, ea.DeviceID); n != 1 {
		t.Fatal("Instance device keeps the old display name")
	}
	for _, name := range []string{"Rename-WorkStation", "rename-workstation"} {
		if code := statusOf(b.send("PATCH", "/api/hosts/"+other.ID, `{"name":"`+name+`"}`)); code != http.StatusConflict {
			t.Fatalf("clashing name %q: %d, want 409", name, code)
		}
	}
	if code := statusOf(b.send("PATCH", "/api/hosts/"+a.ID, `{"name":"rename-workstation"}`)); code != http.StatusNoContent {
		t.Fatalf("renaming to its own name: %d, want 204", code)
	}
	for _, bad := range []string{`{"name":""}`, `{"name":"has space"}`, `{"name":"` + strings.Repeat("x", 65) + `"}`, `{}`, `nope`} {
		if code := statusOf(b.send("PATCH", "/api/hosts/"+a.ID, bad)); code != http.StatusBadRequest {
			t.Fatalf("payload %s: %d, want 400", bad, code)
		}
	}

	c := plainUser.browser(t)
	if code := statusOf(c.send("PATCH", "/api/hosts/"+a.ID, `{"name":"mine-now"}`)); code != http.StatusNotFound {
		t.Fatalf("another User's Host: %d, want 404", code)
	}
	if code := statusOf(newBrowser(t).send("PATCH", "/api/hosts/"+a.ID, `{"name":"x"}`)); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d, want 401", code)
	}
}

func hostsRevoke(t *testing.T, f *fx) {
	b := f.b
	me, _ := b.me()
	h, e := enrolled(t, b, "revoke-laptop")
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusOK {
		t.Fatalf("before revoke: %d", code)
	}

	if code := statusOf(b.post("/api/hosts/" + h.ID + "/revoke")); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusUnauthorized {
		t.Fatalf("revoked Credential: %d, want 401", code)
	}
	got, _ := b.hostByID(h.ID)
	if got.Status != "revoked" || got.RevokedAt == nil {
		t.Fatalf("after revoke %+v", got)
	}
	if n := instanceCount(t, me.ID, `select count(*) from agentsview.raw_devices where device_id = $1 and revoked_at is not null`, e.DeviceID); n != 1 {
		t.Fatal("Instance device not revoked")
	}
	if b.findHost("revoke-laptop") == nil {
		t.Fatal("revoked Host dropped from the list; the UI filters it")
	}
	if code := statusOf(b.post("/api/hosts/" + h.ID + "/revoke")); code != http.StatusNoContent {
		t.Fatalf("second revoke: %d, want idempotent 204", code)
	}
	if code := statusOf(b.send("PATCH", "/api/hosts/"+h.ID, `{"name":"back"}`)); code != http.StatusConflict {
		t.Fatalf("renaming a revoked Host: %d, want 409", code)
	}
	if n := len(b.hostsNamed("revoke-laptop")); n != 1 {
		t.Fatalf("Hosts %d", n)
	}
	if again := mustEnrol(t, b.newToken().Token, "revoke-laptop"); again.Name != "revoke-laptop" {
		t.Fatalf("reused name became %q", again.Name)
	}

	c := plainUser.browser(t)
	if code := statusOf(c.post("/api/hosts/" + h.ID + "/revoke")); code != http.StatusNotFound {
		t.Fatalf("another User's Host: %d, want 404", code)
	}
}

func hostsDelete(t *testing.T, f *fx) {
	b := f.b
	me, _ := b.me()
	h, e := enrolled(t, b, "delete-laptop")

	if code := statusOf(plainUser.browser(t).send("DELETE", "/api/hosts/"+h.ID, "")); code != http.StatusNotFound {
		t.Fatalf("another User's Host: %d, want 404", code)
	}
	if code := statusOf(b.send("DELETE", "/api/hosts/"+h.ID, "")); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if _, code := b.hostByID(h.ID); code != http.StatusNotFound {
		t.Fatalf("deleted Host detail: %d, want 404", code)
	}
	if b.findHost("delete-laptop") != nil {
		t.Fatal("deleted Host still listed")
	}
	if code := statusOf(rawSyncToken(t, e.UserPath, e)); code != http.StatusUnauthorized {
		t.Fatalf("deleted Host's Credential: %d, want 401", code)
	}
	if n := instanceCount(t, me.ID, `select count(*) from agentsview.raw_devices where device_id = $1 and revoked_at is not null`, e.DeviceID); n != 1 {
		t.Fatal("Instance device not revoked")
	}
	if code := statusOf(b.send("DELETE", "/api/hosts/"+h.ID, "")); code != http.StatusNotFound {
		t.Fatalf("second delete: %d, want 404", code)
	}

	r, _ := enrolled(t, b, "delete-revoked")
	b.post("/api/hosts/" + r.ID + "/revoke")
	if code := statusOf(b.send("DELETE", "/api/hosts/"+r.ID, "")); code != http.StatusNoContent {
		t.Fatalf("delete revoked Host: %d", code)
	}
	if b.findHost("delete-revoked") != nil {
		t.Fatal("deleted revoked Host still listed")
	}
}

func checkin(t *testing.T, e enrolReply, version string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", platformURL+"/api/hosts/checkin", strings.NewReader(`{"version":"`+version+`"}`))
	req.Header.Set("Authorization", "Bearer "+e.Credential)
	req.Header.Set("X-AgentsView-Device-ID", e.DeviceID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return statusOf(resp)
}

func hostsVersion(t *testing.T, f *fx) {
	b := f.b
	h, e := enrolled(t, b, "version-box")
	pinned := b.hosts().PinnedVersion
	if h.Outdated {
		t.Fatalf("Host enrolled at the pinned version is outdated: %+v", h)
	}
	if code := checkin(t, e, "0.40.1"); code != http.StatusNoContent {
		t.Fatalf("checkin: %d", code)
	}
	if got, _ := b.hostByID(h.ID); got.Version != "0.40.1" || !got.Outdated || got.Status != "online" {
		t.Fatalf("after reporting an old version: %+v", got)
	}
	if code := checkin(t, e, pinned); code != http.StatusNoContent {
		t.Fatalf("checkin: %d", code)
	}
	if got, _ := b.hostByID(h.ID); got.Outdated || got.Version != pinned {
		t.Fatalf("after upgrading: %+v", got)
	}
	if code := checkin(t, enrolReply{DeviceID: e.DeviceID, Credential: "avdc_wrong"}, "0.1.0"); code != http.StatusUnauthorized {
		t.Fatalf("wrong Credential: %d, want 401", code)
	}
	if code := checkin(t, e, "bad version"); code != http.StatusBadRequest {
		t.Fatalf("malformed version: %d, want 400", code)
	}
	b.post("/api/hosts/" + h.ID + "/revoke").Body.Close()
	if code := checkin(t, e, pinned); code != http.StatusUnauthorized {
		t.Fatalf("revoked Host checking in: %d, want 401", code)
	}
}

func hostsDetail(t *testing.T, f *fx) {
	b := f.b
	h, _ := enrolled(t, b, "detail-box")
	d := decode[struct {
		Uninstall string `json:"uninstallCommand"`
	}](t, b.get("/api/hosts/"+h.ID))
	if want := "curl -fsSL " + b.newToken().BaseURL + "/uninstall/linux.sh | bash"; d.Uninstall != want {
		t.Fatalf("uninstall command %q, want %q", d.Uninstall, want)
	}
	if code := statusOf(b.get("/api/hosts/00000000-0000-0000-0000-000000000000")); code != http.StatusNotFound {
		t.Fatalf("unknown Host: %d", code)
	}
	if code := statusOf(b.get("/api/hosts/not-a-uuid")); code != http.StatusNotFound {
		t.Fatalf("malformed id: %d", code)
	}
}
