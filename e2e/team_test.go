//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type teamTotals struct {
	Sessions    int64   `json:"sessions"`
	TokensIn    int64   `json:"tokensIn"`
	TokensOut   int64   `json:"tokensOut"`
	CostUSD     float64 `json:"costUsd"`
	ActiveUsers int     `json:"activeUsers"`
	ActiveHosts int     `json:"activeHosts"`
	Daily       []struct {
		Date        string  `json:"date"`
		Sessions    int64   `json:"sessions"`
		Tokens      int64   `json:"tokens"`
		Cost        float64 `json:"cost"`
		ActiveUsers int     `json:"activeUsers"`
		ActiveHosts int     `json:"activeHosts"`
	} `json:"daily"`
	Models []struct {
		Name     string `json:"name"`
		Sessions int64  `json:"sessions"`
		Tokens   int64  `json:"tokens"`
	} `json:"models"`
	Agents []struct {
		Name     string `json:"name"`
		Sessions int64  `json:"sessions"`
	} `json:"agents"`
}

type teamReply struct {
	Org teamTotals `json:"org"`
	You teamTotals `json:"you"`
}

func (b *browser) team() (teamReply, string) {
	b.t.Helper()
	resp := b.get("/api/team")
	if resp.StatusCode != http.StatusOK {
		b.t.Fatalf("team: %d %s", resp.StatusCode, body(b.t, resp))
	}
	raw := body(b.t, resp)
	var out teamReply
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		b.t.Fatal(err)
	}
	return out, raw
}

type breakdownRow struct {
	ID         string  `json:"id"`
	Sessions   int64   `json:"sessions"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`
	Hosts      int     `json:"hosts"`
	LastActive *string `json:"lastActive"`
}

func breakdownOf(b *browser, id string) *breakdownRow {
	info := decode[struct {
		Users []breakdownRow `json:"users"`
	}](b.t, b.get("/api/admin/instance"))
	for _, u := range info.Users {
		if u.ID == id {
			return &u
		}
	}
	return nil
}

// machineUsage counts the sessions an Instance's usage figures see (all, or one device label);
// usage lags the session list, so figures are only stable once this matches.
func machineUsage(b *browser, base, label string) int64 {
	resp := b.get(base + "api/v1/usage/summary?from=2020-01-01&to=2100-01-01&session_counts=true&timezone=UTC&machine=" + url.QueryEscape(label))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return -1
	}
	return decode[struct {
		SessionCounts struct {
			Total int64 `json:"total"`
		} `json:"sessionCounts"`
	}](b.t, resp).SessionCounts.Total
}

func uploader(t *testing.T, m *machine, b *browser, email string, salt, from, to int) (*login, string) {
	t.Helper()
	l := m.newLogin(t)
	l.salt = salt
	l.writeSessionRange(from, to)
	tok := b.newToken()
	l.mustSh(installCommand(tok, "--profile"))
	_, host := b.tokenState(tok.ID)
	if host == nil {
		t.Fatal("no Host")
	}
	return l, email + "/" + host.Name
}

// Serial: it moves the test clock. The Admin is its own User so the per-User breakdown counts only
// this test's uploads.
func TestTeam(t *testing.T) {
	admin, adminEmail := adminSignedIn(t, "teamadmin")
	admin.client.Timeout = 90 * time.Second // the Admin Instance is slow while it replays earlier tests
	t.Run("Team page groups figures, never per person, and serves from the cache", func(t *testing.T) {
		teamPage(t, admin.with(t), adminEmail)
	})
	t.Run("a new User is served empty figures at once, without waiting on upstream", func(t *testing.T) {
		newcomer, email := signedIn(t, "newcomer")
		start := time.Now()
		reply, _ := newcomer.team()
		if took := time.Since(start); took > 3*time.Second {
			t.Fatalf("Team page took %s", took)
		}
		if reply.You.Sessions != 0 || len(reply.You.Daily) != 30 {
			t.Fatalf("new User's figures: %+v", reply.You)
		}
		id := userID(t, admin.with(t), email)
		if row := breakdownOf(admin.with(t), id); row == nil || row.Sessions != 0 {
			t.Fatalf("new User's breakdown row: %+v", row)
		}
		refreshStats(t)
		if row := breakdownOf(admin.with(t), id); row == nil {
			t.Fatal("breakdown row lost after refresh")
		}
	})
	t.Run("storage shows per User on the Users list", func(t *testing.T) {
		admin := admin.with(t)
		admin.waitInstance("running")
		eventually(t, "storageBytes > 0", 90*time.Second, func() bool {
			refreshStats(t)
			_, list := call(admin, "GET", "/api/admin/users", nil)
			for _, u := range list["users"].([]any) {
				if u := u.(map[string]any); u["email"] == adminEmail {
					return num(u, "storageBytes") > 0
				}
			}
			return false
		})
	})
}

func teamPage(t *testing.T, admin *browser, adminEmail string) {
	m, sid := machineFor(t, "linux-profile"), freshSessionID()
	other, otherEmail := signedIn(t, "teammate")
	other.client.Timeout = 90 * time.Second
	admin.waitInstance("running")
	otherBase := other.waitInstance("running").URL
	_, adminLabel := uploader(t, m, admin, adminEmail, sid, 0, 2)
	l2, otherLabel := uploader(t, m, other, otherEmail, sid+1000, 3, 4)
	eventually(t, "Admin Instance running", 120*time.Second, func() bool { return admin.adminInstance().State == "running" })
	adminBase := admin.adminInstance().URL
	eventually(t, "sessions replayed", 420*time.Second, func() bool {
		return machineUsage(admin, adminBase, adminLabel) == 2 && machineUsage(admin, adminBase, otherLabel) == 1
	})
	eventually(t, "teammate's own Instance parsed", 120*time.Second, func() bool { return sessionCount(other, otherBase) == 1 })

	refreshStats(t)
	adminID, otherID := userID(t, admin, adminEmail), userID(t, admin, otherEmail)
	a, b := breakdownOf(admin, adminID), breakdownOf(admin, otherID)
	if a == nil || b == nil {
		t.Fatalf("breakdown missing a User: %+v %+v", a, b)
	}
	if a.Sessions != 2 || b.Sessions != 1 || a.Hosts != 1 || b.Hosts != 1 || a.Tokens != 2*b.Tokens || b.Tokens == 0 || a.LastActive == nil {
		t.Fatalf("per-User breakdown wrong: %+v %+v", a, b)
	}

	first, raw := other.team()
	if first.You.Sessions != 1 || first.You.TokensOut != 900 || first.You.TokensIn <= first.You.TokensOut {
		t.Fatalf("own figures: %+v", first.You)
	}
	if first.Org.Sessions < 3 || first.Org.TokensOut < 2700 || first.Org.ActiveUsers < 2 || first.Org.ActiveHosts < 2 || len(first.Org.Daily) != 30 {
		t.Fatalf("org figures: %+v", first.Org)
	}
	var sessions, tokens int64
	var users, hosts int
	for _, d := range first.Org.Daily {
		sessions, tokens = sessions+d.Sessions, tokens+d.Tokens
		users, hosts = max(users, d.ActiveUsers), max(hosts, d.ActiveHosts)
	}
	if sessions < first.Org.Sessions || tokens != first.Org.TokensIn+first.Org.TokensOut || users < 2 || hosts < 2 {
		t.Fatalf("daily series sessions %d tokens %d active Users %d Hosts %d: %+v", sessions, tokens, users, hosts, first.Org)
	}
	if !strings.Contains(raw, `"cost"`) {
		t.Fatalf("daily series has no cost: %s", raw)
	}
	if len(first.Org.Models) == 0 || len(first.Org.Agents) == 0 || first.Org.Agents[0].Name == "" {
		t.Fatalf("top models and agents missing: %+v", first.Org)
	}
	for _, leak := range []string{adminEmail, otherEmail, adminID, otherID, adminLabel, otherLabel, `"users"`} {
		if strings.Contains(raw, leak) {
			t.Fatalf("Team page names a person or device (%q): %s", leak, raw)
		}
	}
	if resp := newBrowser(t).get("/api/team"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", resp.StatusCode)
	}

	l2.writeSessionRange(4, 5)
	eventually(t, "second session replayed", 420*time.Second, func() bool {
		return machineUsage(admin, adminBase, otherLabel) == 2 && machineUsage(other, otherBase, "") == 2
	})
	if again, _ := other.team(); again.You.Sessions != 1 || again.Org.Sessions != first.Org.Sessions {
		t.Fatalf("figures changed between refreshes: %+v", again)
	}
	if got := breakdownOf(admin, otherID); got.Sessions != 1 {
		t.Fatalf("breakdown changed between refreshes: %+v", got)
	}

	refreshStats(t)
	fresh, _ := other.team()
	if fresh.You.Sessions != 2 || fresh.Org.Sessions != first.Org.Sessions+1 {
		t.Fatalf("figures not refreshed: before %+v after %+v", first.Org, fresh.Org)
	}
	if got := breakdownOf(admin, otherID); got.Sessions != 2 {
		t.Fatalf("breakdown not refreshed: %+v", got)
	}
}

// freshSessionID is a session index no other test uses: upstream counts one session id from
// several devices once, so shared ids would be credited to whichever device came first.
func freshSessionID() int { return int(time.Now().UnixNano() / 1e3 % 900_000_000) }
