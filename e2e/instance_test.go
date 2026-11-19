//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type instanceStatus struct {
	State   string `json:"state"`
	URL     string `json:"url"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

func (b *browser) instance() (instanceStatus, int) {
	b.t.Helper()
	resp := b.get("/api/instance")
	defer resp.Body.Close()
	var s instanceStatus
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			b.t.Fatal(err)
		}
	}
	return s, resp.StatusCode
}

func (b *browser) waitInstance(state string) instanceStatus {
	b.t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	var failedSince time.Time
	for {
		s, code := b.instance()
		if code == http.StatusOK && s.State == state {
			return s
		}
		// Failed is terminal until a retry, which applies asynchronously; a lasting failure will not recover.
		if code == http.StatusOK && s.State == "failed" {
			if failedSince.IsZero() {
				failedSince = time.Now()
			} else if time.Since(failedSince) > 10*time.Second {
				b.t.Fatalf("instance failed instead of reaching %q (%+v)", state, s)
			}
		} else {
			failedSince = time.Time{}
		}
		if time.Now().After(deadline) {
			b.t.Fatalf("instance never reached %q (last %d %+v)", state, code, s)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func statusOf(resp *http.Response) int {
	resp.Body.Close()
	return resp.StatusCode
}

// running signs a fresh User in and waits for their Instance.
func running(t *testing.T, prefix string) (b *browser, email, base string) {
	t.Helper()
	b, email = signedIn(t, prefix)
	return b, email, b.waitInstance("running").URL
}

func eventually(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestInstanceProxy(t *testing.T) {
	t.Parallel()
	f := newFx(t, "proxy")
	f.run(t, "first sign-in provisions an Instance served with the Platform bar", proxyProvisioned)
	f.run(t, "never hands out the bearer token", proxyNoBearerLeak)
	f.run(t, "only the owner reaches an Instance", proxyOwnerOnly)
	f.run(t, "events request reaches the Instance with the bearer added", proxyEvents)
	f.run(t, "status needs sign-in", proxyStatusNeedsSignIn)
	f.run(t, "crashed Instance is restarted", proxyCrashRestart)
}

func proxyStatusNeedsSignIn(t *testing.T, _ *fx) {
	if _, code := newBrowser(t).instance(); code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", code)
	}
}

func proxyProvisioned(t *testing.T, f *fx) {
	b, base := f.b, f.base
	m, _ := b.me()
	if base != "/i/"+m.ID+"/" {
		t.Fatalf("instance url %q, want /i/%s/", base, m.ID)
	}

	page := b.get(base)
	status := page.StatusCode
	html := body(t, page)
	if status != http.StatusOK || !strings.Contains(html, `<base href="`+base+`"`) {
		t.Fatalf("agentsview page: %d %.200s", status, html)
	}
	if !strings.Contains(html, `id="av-platbar"`) || !strings.Contains(html, "/_platform/bar.js") {
		t.Fatalf("Platform bar not injected: %.400s", html)
	}
	// agentsview's CSP forbids inline script, so the bar's script is served same-origin.
	if code := statusOf(b.get("/_platform/bar.js")); code != http.StatusOK {
		t.Fatalf("bar.js: %d", code)
	}

	if resp := b.get(base + "api/v1/stats"); resp.StatusCode != http.StatusOK {
		t.Fatalf("stats through proxy: %d %s", resp.StatusCode, body(t, resp))
	} else {
		resp.Body.Close()
	}

	if n := count(t, `select count(*) from instances i join pg_database d on d.datname = i.db_name where i.user_id = $1::uuid`, m.ID); n != 1 {
		t.Fatal("instance database missing")
	}
	if n := count(t, `select count(*) from instances i join pg_roles r on r.rolname like i.db_name || '\_%' where i.user_id = $1::uuid`, m.ID); n != 2 {
		t.Fatalf("want owner and runtime roles, got %d", n)
	}
	if n := count(t, `
		select count(*) from instances where user_id = $1::uuid
		and get_byte(bearer_token, 0) = 1 and get_byte(owner_password, 0) = 1
		and get_byte(runtime_password, 0) = 1 and get_byte(cursor_secret, 0) = 1
		and length(bearer_token) > 28 + 32`, m.ID); n != 1 {
		t.Fatal("secrets are not sealed with the Platform key")
	}
}

func proxyNoBearerLeak(t *testing.T, f *fx) {
	b, base := f.b, f.base
	// Upstream returns the bearer to loopback callers that send no X-Forwarded-For.
	resp := b.get(base + "api/v1/settings")
	status := resp.StatusCode
	raw := body(t, resp)
	if status != http.StatusOK {
		t.Fatalf("settings: %d %s", status, raw)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	if tok, _ := s["auth_token"].(string); tok != "" {
		t.Fatal("Instance bearer token leaked by GET settings")
	}
}

func proxyOwnerOnly(t *testing.T, f *fx) {
	owner, base := f.b, f.base
	other := plainUser.browser(t)

	if code := statusOf(other.get(base)); code != http.StatusForbidden {
		t.Fatalf("other User: %d, want 403", code)
	}
	if code := statusOf(other.get(base + "api/v1/stats")); code != http.StatusForbidden {
		t.Fatalf("other User API: %d, want 403", code)
	}
	if code := statusOf(newBrowser(t).get(base + "api/v1/stats")); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d, want 401", code)
	}
	if code := statusOf(owner.get(base + "api/v1/stats")); code != http.StatusOK {
		t.Fatalf("owner: %d", code)
	}
	resp := owner.get("/i/me/")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != base {
		t.Fatalf("/i/me/ redirects to %q, want %q", loc, base)
	}
}

func rawSyncUnknown(t *testing.T, base string) func() *http.Response {
	return func() *http.Response {
		req, _ := http.NewRequest("POST", platformURL+base+"api/v1/raw-sync/tokens", strings.NewReader(`{"scopes":["upload"]}`))
		req.Header.Set("Authorization", "Bearer avdc_unknown-credential")
		req.Header.Set("X-AgentsView-Device-ID", "dev_unknown")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
}

// Hosted agentsview has no event stream, so it answers 503 itself; that answer,
// rather than a 401, shows the request got through with the bearer added.
func proxyEvents(t *testing.T, f *fx) {
	b, base := f.b, f.base
	resp := b.get(base + "api/v1/events")
	status := resp.StatusCode
	raw := body(t, resp)
	if status != http.StatusServiceUnavailable || !strings.Contains(raw, "events not available in this mode") {
		t.Fatalf("events: %d %s", status, raw)
	}
}

func pid(t *testing.T, b *browser, base string) int {
	t.Helper()
	resp := b.get(base + "api/ping")
	status := resp.StatusCode
	raw := body(t, resp)
	if status != http.StatusOK {
		return 0
	}
	var p struct {
		Pid int `json:"pid"`
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return 0
	}
	return p.Pid
}

func testPost(t *testing.T, path, payload string) {
	t.Helper()
	resp, err := http.Post(platformURL+path, "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("%s: %d", path, resp.StatusCode)
	}
}

func proxyCrashRestart(t *testing.T, f *fx) {
	b, base := f.b, f.base
	m, _ := b.me()
	before := pid(t, b, base)
	if before == 0 {
		t.Fatal("no pid before kill")
	}
	testPost(t, "/_test/instances/"+m.ID+"/kill", "")
	eventually(t, "Instance restarted with a new pid", 60*time.Second, func() bool {
		after := pid(t, b, base)
		return after != 0 && after != before
	})
}

func TestInstanceRecovery(t *testing.T) {
	t.Parallel()
	f := newFx(t, "recovery")
	f.run(t, "provisioning resumes after a partial run", recoveryPartialRun)
	f.run(t, "failing Instance is marked failed until retried", recoveryRetry)
	f.run(t, "retry in Instance View restarts the viewed User's Instance", recoveryRetryInView)
}

func recoveryPartialRun(t *testing.T, f *fx) {
	b := f.b
	m, _ := b.me()
	sql(t, "drop database av_"+strings.ReplaceAll(m.ID, "-", "")+" with (force)")
	testPost(t, "/_test/instances/"+m.ID+"/kill", "")

	eventually(t, "Instance back on a re-provisioned database", 90*time.Second, func() bool {
		return statusOf(b.get(f.base+"api/v1/stats")) == http.StatusOK
	})
	again := signInAs(t, f.email)
	if m2, _ := again.me(); m2.ID != m.ID {
		t.Fatal("second sign-in created another User")
	}
	again.waitInstance("running")
}

func failInstance(t *testing.T, b *browser, id string) {
	t.Helper()
	testPost(t, "/_test/instances/"+id+"/sabotage", `{"on":true}`)
	testPost(t, "/_test/instances/"+id+"/kill", "")
	b.waitInstance("failed")
}

func recoveryRetry(t *testing.T, f *fx) {
	b := f.b
	m, _ := b.me()
	failInstance(t, b, m.ID)
	if s, _ := b.instance(); s.Error != "instance_failed" {
		t.Fatalf("error %q, want the code instance_failed", s.Error)
	}

	testPost(t, "/_test/instances/"+m.ID+"/sabotage", `{"on":false}`)
	if code := statusOf(b.post("/api/instance/retry")); code != http.StatusAccepted {
		t.Fatalf("retry: %d", code)
	}
	b.waitInstance("running")
}

func recoveryRetryInView(t *testing.T, f *fx) {
	user := f.b
	m, _ := user.me()
	admin := adminUser.browser(t)
	failInstance(t, user, m.ID)
	testPost(t, "/_test/instances/"+m.ID+"/sabotage", `{"on":false}`)

	if code := statusOf(admin.post("/api/instance/retry?user=" + f.id())); code != http.StatusAccepted {
		t.Fatalf("retry in Instance View: %d", code)
	}
	user.waitInstance("running")

	if code := statusOf(user.post("/api/instance/retry?user=" + f.id())); code != http.StatusAccepted {
		t.Fatalf("a User retrying their own Instance by id: %d", code)
	}
	other := plainUser.browser(t)
	if code := statusOf(other.post("/api/instance/retry?user=" + f.id())); code != http.StatusForbidden {
		t.Fatalf("non-Admin retrying someone else's Instance: %d, want 403", code)
	}
}

func TestOverviewServesTheLastUsageWhileTheInstanceIsDown(t *testing.T) {
	t.Parallel()
	f := newFx(t, "usage-cache")
	b := f.b
	m, _ := b.me()
	b.waitInstance("running")
	type reply struct {
		Usage      *struct{} `json:"usage"`
		UsageStale bool      `json:"usageStale"`
	}
	eventually(t, "Overview usage from the Instance", 60*time.Second, func() bool {
		return decode[reply](t, b.get("/api/overview")).Usage != nil
	})
	const pastUsageTTL = 11 * time.Second
	time.Sleep(pastUsageTTL)

	failInstance(t, b, m.ID)
	got := decode[reply](t, b.get("/api/overview"))
	if got.Usage == nil || !got.UsageStale {
		t.Fatalf("Overview while the Instance is down: %+v, want the last usage marked stale", got)
	}
	testPost(t, "/_test/instances/"+m.ID+"/sabotage", `{"on":false}`)
}
