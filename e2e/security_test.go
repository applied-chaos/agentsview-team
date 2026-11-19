//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func adminCrossSite(t *testing.T, target, _ *fx) {
	admin := adminUser.browser(t)
	id := target.id()

	for name, hdr := range map[string]map[string]string{
		"cross-site fetch":  {"Sec-Fetch-Site": "cross-site"},
		"same-site sibling": {"Sec-Fetch-Site": "same-site"},
		"foreign origin":    {"Origin": "https://evil.example"},
	} {
		admin.headers = hdr
		if code := impersonate(admin, id); code != http.StatusForbidden {
			t.Errorf("%s: impersonate %d, want 403", name, code)
		}
		if resp, _ := call(admin, "POST", "/api/admin/users/"+id+"/disable", nil); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: disable %d, want 403", name, resp.StatusCode)
		}
	}
	if v := meAs(t, admin); v.ImpersonatedBy != nil {
		t.Fatal("a cross-site request started an Impersonation")
	}

	admin.headers = map[string]string{"Sec-Fetch-Site": "same-origin"}
	if code := impersonate(admin, id); code != http.StatusNoContent {
		t.Fatalf("same-origin impersonate: %d, want 204", code)
	}
	admin.headers = nil
	stopImpersonating(admin)
}

func TestPlatformPagesAreNotFrameableAndCarryACSP(t *testing.T) {
	t.Parallel()
	resp := newBrowser(t).get("/sign-in")
	resp.Body.Close()
	h := resp.Header
	if h.Get("X-Frame-Options") != "DENY" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", h)
	}
	csp := h.Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "script-src 'self'") ||
		!strings.Contains(csp, "img-src 'self' data: https://www.gravatar.com") {
		t.Fatalf("CSP %q", csp)
	}
}
