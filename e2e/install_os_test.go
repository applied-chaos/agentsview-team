//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func hostsInstallScripts(t *testing.T, f *fx) {
	b := f.b
	pinned := b.hosts().PinnedVersion
	for _, c := range []struct{ path, ctype, want string }{
		{"/install/macos.sh", "text/x-shellscript", "launchd"},
		{"/install/macos-uninstall.sh", "text/x-shellscript", "launchctl"},
		{"/install/windows.ps1", "text/plain", "Register-ScheduledTask"},
		{"/install/windows-uninstall.ps1", "text/plain", "Unregister-ScheduledTask"},
	} {
		resp, err := http.Get(platformURL + c.path)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		got := string(raw)
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), c.ctype) {
			t.Fatalf("%s: %d %q", c.path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		if !strings.Contains(got, c.want) || strings.Contains(got, "__PLATFORM_URL__") || strings.Contains(got, "__VERSION__") {
			t.Fatalf("%s: wanted %q and no placeholders", c.path, c.want)
		}
		if !strings.Contains(c.path, "uninstall") && !strings.Contains(got, "'"+pinned+"'") && !strings.Contains(got, `"`+pinned+`"`) {
			t.Fatalf("%s does not pin version %s", c.path, pinned)
		}
	}
}

func hostsOSModes(t *testing.T, f *fx) {
	b := f.b
	for _, c := range []struct{ mode, osName, uninstall, update string }{
		{"launchd", "macOS 15.1", "curl -fsSL %s/install/macos-uninstall.sh | bash", "curl -fsSL %s/install/macos.sh | bash"},
		{"task", "Windows 11 Pro", "irm %s/install/windows-uninstall.ps1 | iex", "irm %s/install/windows.ps1 | iex"},
	} {
		mode, osName := c.mode, c.osName
		tok := b.newToken()
		payload, _ := json.Marshal(map[string]string{"token": tok.Token, "name": "dev-" + mode, "os": osName, "mode": mode, "version": "0.47.0"})
		resp, err := http.Post(platformURL+"/api/hosts/enrol", "application/json", strings.NewReader(string(payload)))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s enrol: %d %s", mode, resp.StatusCode, body(t, resp))
		}
		resp.Body.Close()
		h := b.findHost("dev-" + mode)
		if h == nil || h.InstallMode != mode || h.OS != osName {
			t.Fatalf("%s: Host %+v", mode, h)
		}
		d := decode[struct {
			Uninstall string `json:"uninstallCommand"`
			Update    string `json:"updateCommand"`
		}](t, b.get("/api/hosts/"+h.ID))
		if d.Uninstall != fmt.Sprintf(c.uninstall, tok.BaseURL) || d.Update != fmt.Sprintf(c.update, tok.BaseURL) {
			t.Fatalf("%s: commands %+v", mode, d)
		}
	}
}

func tokensEphemeralTick(t *testing.T, f *fx) {
	b := f.b
	tok := b.mustCreateToken(tokenOpts{Ephemeral: true})
	if tok.Reusable || !tok.Ephemeral || tok.ExpiresAt.Sub(tok.CreatedAt) != time.Hour {
		t.Fatalf("token %+v, want single-use Ephemeral valid for 1 hour", tok)
	}
	mustEnrol(t, tok.Token, "ticked")
	if h := b.findHost("ticked"); h == nil || !h.Ephemeral {
		t.Fatalf("Host %+v, want Ephemeral", h)
	}
}
