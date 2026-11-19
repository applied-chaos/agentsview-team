//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func endedEntries(t *testing.T, reader *browser, name string) []map[string]any {
	t.Helper()
	var ended []map[string]any
	for _, e := range auditEntries(t, reader, "type=impersonation&q="+name) {
		if e["action"] == "Impersonation ended" {
			ended = append(ended, e)
		}
	}
	return ended
}

// Serial: it moves the test clock and Deletes a target. The Admins are sessions of the shared Admin.
func TestImpersonationEndedByExpiryDisableOrDeleteIsAudited(t *testing.T) {
	_, expEmail, expBase := running(t, "expirer")
	expName := rename(t, expEmail)
	expAdmin := adminUser.browser(t)
	_, offEmail, offBase := running(t, "disabled-target")
	offName := rename(t, offEmail)
	offAdmin := adminUser.browser(t)
	_, delEmail, delBase := running(t, "deleted-target")
	delName := rename(t, delEmail)
	delAdmin := adminUser.browser(t)
	for _, c := range []struct {
		b    *browser
		base string
	}{{expAdmin, expBase}, {offAdmin, offBase}, {delAdmin, delBase}} {
		if impersonate(c.b, idOf(c.base)) != http.StatusNoContent {
			t.Fatal("impersonate failed")
		}
	}

	actor := adminUser.browser(t)
	if resp, _ := call(actor, "POST", "/api/admin/users/"+idOf(offBase)+"/disable", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("disable: %d", resp.StatusCode)
	}
	if resp, _ := call(actor, "POST", "/api/admin/users/"+idOf(delBase)+"/delete", map[string]string{"confirm": delEmail}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	for _, name := range []string{offName, delName} {
		got := endedEntries(t, actor, name)
		if len(got) != 1 || !strings.Contains(got[0]["details"].(string), "disabled") {
			t.Fatalf("%s: ended entries %v, want one mentioning disabled", name, got)
		}
	}

	advance(t, 8*24*time.Hour)
	if refresh(expAdmin) != http.StatusUnauthorized {
		t.Fatal("expired session refreshed")
	}
	reader := adminUser.browser(t)
	got := endedEntries(t, reader, expName)
	if len(got) != 1 || !strings.Contains(got[0]["details"].(string), "expired") {
		t.Fatalf("expiry: ended entries %v", got)
	}
}

func impersonationEverythingAudited(t *testing.T, target, _ *fx, name string) {
	admin := adminUser.browser(t)
	before := len(auditEntries(t, admin, "type=impersonation&q="+name))
	if impersonate(admin, idOf(target.base)) != http.StatusNoContent {
		t.Fatal("impersonate failed")
	}
	tok := admin.mustCreateToken(tokenOpts{Description: "while impersonating"})
	if code := statusOf(admin.post("/api/hosts/tokens/" + tok.ID + "/revoke")); code != http.StatusNoContent {
		t.Fatalf("revoke token: %d", code)
	}
	h, _ := enrolled(t, admin, "imp-delete-host")
	if code := statusOf(admin.send("DELETE", "/api/hosts/"+h.ID, "")); code != http.StatusNoContent {
		t.Fatalf("delete Host: %d", code)
	}
	admin.post(target.base + "api/v1/sessions/batch-delete").Body.Close()
	stopImpersonating(admin)

	seen := map[any]bool{}
	for _, e := range auditSince(t, admin, "type=impersonation&q="+name, before) {
		seen[e["action"]] = true
	}
	for _, want := range []string{
		"Install Token created while impersonating", "Install Token revoked while impersonating", "Instance changed while impersonating",
		"Host deleted while impersonating",
	} {
		if !seen[want] {
			t.Errorf("missing audit entry %q (have %v)", want, seen)
		}
	}
}
