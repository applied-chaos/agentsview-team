//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type tokenReply struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	BaseURL string `json:"baseUrl"`
}

type hostView struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	OS           string  `json:"os"`
	InstallMode  string  `json:"installMode"`
	Status       string  `json:"status"`
	LastSeenAt   *string `json:"lastSeenAt"`
	LastUploadAt *string `json:"lastUploadAt"`
	Version      string  `json:"version"`
	Outdated     bool    `json:"outdated"`
	Ephemeral    bool    `json:"ephemeral"`
	RevokedAt    *string `json:"revokedAt"`
}

type hostsReply struct {
	Hosts         []hostView `json:"hosts"`
	PinnedVersion string     `json:"pinnedVersion"`
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode (status %d): %v", resp.StatusCode, err)
	}
	return v
}

func (b *browser) newToken() tokenReply {
	b.t.Helper()
	resp := b.post("/api/hosts/tokens")
	if resp.StatusCode != http.StatusCreated {
		b.t.Fatalf("new token: %d %s", resp.StatusCode, body(b.t, resp))
	}
	return decode[tokenReply](b.t, resp)
}

func (b *browser) hosts() hostsReply {
	b.t.Helper()
	resp := b.get("/api/hosts")
	if resp.StatusCode != http.StatusOK {
		b.t.Fatalf("hosts: %d %s", resp.StatusCode, body(b.t, resp))
	}
	return decode[hostsReply](b.t, resp)
}

func (b *browser) tokenState(id string) (state string, host *hostView) {
	b.t.Helper()
	out := decode[struct {
		State string    `json:"state"`
		Host  *hostView `json:"host"`
	}](b.t, b.get("/api/hosts/tokens/"+id))
	return out.State, out.Host
}

type enrolReply struct {
	Name       string `json:"name"`
	UserPath   string `json:"userPath"`
	DeviceID   string `json:"deviceId"`
	Credential string `json:"credential"`
}

func enrolRequest(t *testing.T, token, name string) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"token": token, "name": name, "os": "Debian GNU/Linux 12", "mode": "profile", "version": "0.47.0"})
	resp, err := http.Post(platformURL+"/api/hosts/enrol", "application/json", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func mustEnrol(t *testing.T, token, name string) enrolReply {
	t.Helper()
	resp := enrolRequest(t, token, name)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enrol: %d %s", resp.StatusCode, body(t, resp))
	}
	return decode[enrolReply](t, resp)
}

func rawSyncToken(t *testing.T, userPath string, e enrolReply) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", platformURL+userPath+"/api/v1/raw-sync/tokens", strings.NewReader(`{"scopes":["status"]}`))
	req.Header.Set("Authorization", "Bearer "+e.Credential)
	req.Header.Set("X-AgentsView-Device-ID", e.DeviceID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// instanceCount runs a query in the User's Instance database as a superuser, which bypasses
// upstream's row-level security.
func instanceCount(t *testing.T, userID, query string, args ...any) int {
	t.Helper()
	u, _ := url.Parse(databaseURL)
	u.Path = "/av_" + strings.ReplaceAll(userID, "-", "")
	conn, err := pgx.Connect(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	var n int
	if err := conn.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAddHostTokenIsSingleUseAndExpiresAfterOneHour(t *testing.T) {
	b, _, _ := running(t, "token")
	tok := b.newToken()
	if !strings.HasPrefix(tok.Token, "avt_tk_") || tok.BaseURL == "" {
		t.Fatalf("token reply %+v", tok)
	}
	if state, _ := b.tokenState(tok.ID); state != "waiting" {
		t.Fatalf("fresh token state %q", state)
	}
	mustEnrol(t, tok.Token, "laptop")
	if code := statusOf(enrolRequest(t, tok.Token, "laptop-again")); code != http.StatusUnauthorized {
		t.Fatalf("second use: %d, want 401", code)
	}
	state, host := b.tokenState(tok.ID)
	if state != "connected" || host == nil || host.Name != "laptop" {
		t.Fatalf("token state %q host %+v", state, host)
	}

	late := b.newToken()
	advance(t, 61*time.Minute)
	if code := statusOf(enrolRequest(t, late.Token, "too-late")); code != http.StatusUnauthorized {
		t.Fatalf("expired token: %d, want 401", code)
	}
	refresh(b)
	if state, _ := b.tokenState(late.ID); state != "expired" {
		t.Fatalf("expired token state %q", state)
	}
}

func hostsEnrolment(t *testing.T, f *fx) {
	b := f.b
	m, _ := b.me()
	first := mustEnrol(t, b.newToken().Token, "enrol-build")
	second := mustEnrol(t, b.newToken().Token, "enrol-build")
	if second.Name != "enrol-build-2" {
		t.Fatalf("clashing name became %q, want enrol-build-2", second.Name)
	}
	if first.Credential == second.Credential || first.DeviceID == second.DeviceID || first.UserPath != "/i/"+m.ID {
		t.Fatalf("hosts share secrets or path: %+v %+v", first, second)
	}
	for _, e := range []enrolReply{first, second} {
		if n := instanceCount(t, m.ID, `select count(*) from agentsview.raw_devices where device_id = $1 and display_name = $2
			and credential_sha256 = sha256(convert_to($3, 'UTF8')) and revoked_at is null`, e.DeviceID, e.Name, e.Credential); n != 1 {
			t.Fatalf("device %s not enrolled with its own Credential", e.DeviceID)
		}
		if n := count(t, `select count(*) from hosts where device_id = $1`, e.DeviceID); n != 1 {
			t.Fatal("Host row missing")
		}
		resp := rawSyncToken(t, e.UserPath, e)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Credential refused by the Instance via the Platform: %d %s", resp.StatusCode, body(t, resp))
		}
		resp.Body.Close()
	}
	if got := b.hostsNamed("enrol-build"); len(got) != 2 {
		t.Fatalf("My Hosts lists %d, want 2", len(got))
	}
}

func TestHostIsOnlineForThirtyMinutesAfterContactThenOffline(t *testing.T) {
	b, _, _ := running(t, "online")
	e := mustEnrol(t, b.newToken().Token, "quiet")
	find := func() hostView {
		for _, h := range b.hosts().Hosts {
			if h.Name == "quiet" {
				return h
			}
		}
		t.Fatal("Host missing")
		return hostView{}
	}
	if h := find(); h.Status != "online" || h.OS != "Debian GNU/Linux 12" || h.InstallMode != "profile" || h.LastSeenAt == nil {
		t.Fatalf("new Host %+v", h)
	}
	advance(t, 29*time.Minute)
	refresh(b)
	if h := find(); h.Status != "online" {
		t.Fatalf("after 29 minutes: %s", h.Status)
	}
	advance(t, 2*time.Minute)
	refresh(b)
	if h := find(); h.Status != "offline" {
		t.Fatalf("after 31 minutes: %s", h.Status)
	}
	resp := rawSyncToken(t, e.UserPath, e)
	resp.Body.Close()
	eventually(t, "Host online again after contact", 20*time.Second, func() bool { return find().Status == "online" })
}

func TestHosts(t *testing.T) {
	t.Parallel()
	f := newFx(t, "hosts")
	f.run(t, "enrolment", hostsEnrolment)
	f.run(t, "add host needs sign-in and shows only own hosts", hostsAddNeedsSignIn)
	f.run(t, "rename", hostsRename)
	f.run(t, "revoke", hostsRevoke)
	f.run(t, "delete", hostsDelete)
	f.run(t, "client version and outdated", hostsVersion)
	f.run(t, "detail carries uninstall command", hostsDetail)
	f.run(t, "install scripts per OS", hostsInstallScripts)
	f.run(t, "launchd and task modes", hostsOSModes)
}

func hostsAddNeedsSignIn(t *testing.T, f *fx) {
	if code := statusOf(newBrowser(t).post("/api/hosts/tokens")); code != http.StatusUnauthorized {
		t.Fatalf("signed out token: %d", code)
	}
	mustEnrol(t, f.b.newToken().Token, "a-box")
	stranger := plainUser.browser(t)
	if len(stranger.hosts().Hosts) != 0 {
		t.Fatal("another User's Host is listed")
	}
	tok := f.b.newToken()
	if code := statusOf(stranger.get("/api/hosts/tokens/" + tok.ID)); code != http.StatusNotFound {
		t.Fatalf("another User's token state: %d, want 404", code)
	}
}
