//go:build e2e

package e2e

import (
	"os"
	"testing"
	"time"
)

func spoolObjects(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir("/data/spool/objects")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range entries {
		out[e.Name()] = true
	}
	return out
}

func replaySpoolPruned(t *testing.T, r *replayRun) {
	eventually(t, "sessions in the Instance", 180*time.Second, func() bool { return sessionCount(r.b, r.base) == 2 })
	eventually(t, "sessions in the Admin Instance", 180*time.Second, func() bool { return machineSessions(r.b, r.adminBase, r.label) == 2 })
	eventually(t, "spool objects pruned", 60*time.Second, func() bool {
		for name := range spoolObjects(t) {
			if !r.spoolBefore[name] {
				return false
			}
		}
		return true
	})
}
