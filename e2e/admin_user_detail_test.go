//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func adminUserDetail(t *testing.T, _, subject *fx) {
	user := subject.b
	tok := user.mustCreateToken(tokenOpts{Description: "build servers", Reusable: true, ExpiresIn: "30d"})
	mustEnrol(t, tok.Token, "box-1")
	user.mustCreateToken(tokenOpts{Description: "forever", ExpiresIn: "never"})
	admin := adminUser.browser(t)

	resp := admin.get("/api/admin/users/" + subject.id())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail: %d", resp.StatusCode)
	}
	d := decode[struct {
		Email    string `json:"email"`
		Hosts    int    `json:"hosts"`
		HostList []struct {
			Name string `json:"name"`
		} `json:"hostList"`
		InstallTokens []struct {
			Description  string  `json:"description"`
			Reusable     bool    `json:"reusable"`
			ExpiresAt    *string `json:"expiresAt"`
			HostsCreated int     `json:"hostsCreated"`
		} `json:"installTokens"`
	}](t, resp)
	if d.Email != subject.email || d.Hosts != 1 || len(d.HostList) != 1 || d.HostList[0].Name != "box-1" {
		t.Fatalf("hosts: %+v", d)
	}
	if len(d.InstallTokens) != 2 {
		t.Fatalf("Install Tokens: %+v", d.InstallTokens)
	}
	byName := map[string]int{}
	for i, tk := range d.InstallTokens {
		byName[tk.Description] = i
	}
	build, forever := d.InstallTokens[byName["build servers"]], d.InstallTokens[byName["forever"]]
	if !build.Reusable || build.HostsCreated != 1 || build.ExpiresAt == nil || forever.ExpiresAt != nil {
		t.Fatalf("token fields: %+v %+v", build, forever)
	}
}
