//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func sqlIn(t *testing.T, db, query string) {
	t.Helper()
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + db
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, query); err != nil {
		t.Fatal(err)
	}
}

func adminUserStatus(t *testing.T, admin *browser, id string) string {
	t.Helper()
	resp := admin.get("/api/admin/users")
	defer resp.Body.Close()
	var out struct {
		Users []struct{ ID, Status string } `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, u := range out.Users {
		if u.ID == id {
			return u.Status
		}
	}
	t.Fatalf("user %s not listed", id)
	return ""
}

func upgradeFailure(t *testing.T, admin *browser) {
	victim, _, victimBase := running(t, "upg-victim")
	vm, _ := victim.me()
	bystander, bystanderBase := plainUser.instance(t)

	// An unknown relation makes upstream's hosted-provision refuse to upgrade the schema.
	db := "av_" + strings.ReplaceAll(vm.ID, "-", "")
	sqlIn(t, db, `create table agentsview.rogue (x int)`)
	testPost(t, "/_test/instances/"+vm.ID+"/kill", "")
	failed := victim.waitInstance("failed")

	if failed.Version != "0.47.0" || failed.Error != "instance_failed" {
		t.Fatalf("failed status: %+v", failed)
	}
	if got := adminUserStatus(t, admin, vm.ID); got != "instance_failed" {
		t.Fatalf("Users list status %q, want instance_failed", got)
	}
	resp := admin.get("/api/admin/users/" + vm.ID)
	var detail struct {
		Instance struct{ Status, Version string } `json:"instance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if detail.Instance.Status != "failed" || detail.Instance.Version != "0.47.0" {
		t.Fatalf("detail instance: %+v", detail.Instance)
	}
	if code := statusOf(bystander.get(bystanderBase + "api/v1/stats")); code != http.StatusOK {
		t.Fatalf("other Instance: %d, want 200", code)
	}
	if code := statusOf(victim.get(victimBase + "api/v1/stats")); code != http.StatusServiceUnavailable {
		t.Fatalf("failed Instance: %d, want 503", code)
	}

	sqlIn(t, db, `drop table agentsview.rogue`)
	if code := statusOf(victim.post("/api/instance/retry")); code != http.StatusAccepted {
		t.Fatalf("retry: %d", code)
	}
	victim.waitInstance("running")
	if got := adminUserStatus(t, admin, vm.ID); got != "active" {
		t.Fatalf("Users list status %q after recovery, want active", got)
	}
}
