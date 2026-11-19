//go:build e2e

package e2e

import (
	"testing"
	"time"
)

func num(m map[string]any, key string) float64 {
	n, _ := m[key].(float64)
	return n
}

func TestSpace(t *testing.T) {
	t.Parallel()
	f := newFx(t, "space")

	f.run(t, "a User sees their own space used on the Overview", func(t *testing.T, f *fx) {
		eventually(t, "storageBytes > 0", 90*time.Second, func() bool {
			refreshStats(t)
			_, ov := call(f.b, "GET", "/api/overview", nil)
			return num(ov, "storageBytes") > 0
		})
	})

	f.run(t, "a User gets no Admin space figures", func(t *testing.T, f *fx) {
		for _, path := range []string{"/api/admin/instance", "/api/admin/users"} {
			resp, out := call(f.b, "GET", path, nil)
			if resp.StatusCode != 403 {
				t.Errorf("%s: %d, want 403", path, resp.StatusCode)
			}
			if _, leaked := out["space"]; leaked {
				t.Errorf("%s leaked space", path)
			}
		}
	})

	f.run(t, "an Admin sees the total, the Admin Instance and each User", func(t *testing.T, f *fx) {
		admin := adminUser.browser(t)
		admin.t = t
		admin.waitInstance("running")
		var users, total, inst, spool float64
		eventually(t, "User row and totals", 90*time.Second, func() bool {
			refreshStats(t)
			_, list := call(admin, "GET", "/api/admin/users", nil)
			users = 0
			for _, u := range list["users"].([]any) {
				if u := u.(map[string]any); u["email"] == f.email {
					users = num(u, "storageBytes")
				}
			}
			_, in := call(admin, "GET", "/api/admin/instance", nil)
			sp, _ := in["space"].(map[string]any)
			total, inst, spool = num(sp, "total"), num(sp, "adminInstance"), num(sp, "spool")
			return users > 0 && inst > 0
		})
		if total < users+inst+spool {
			t.Errorf("total %v < User %v + Admin Instance %v + spool %v", total, users, inst, spool)
		}
		_, in := call(admin, "GET", "/api/admin/instance", nil)
		for _, u := range in["users"].([]any) {
			if _, ok := u.(map[string]any)["storageBytes"]; !ok {
				t.Fatalf("usage row without storageBytes: %v", u)
			}
		}
	})
}
