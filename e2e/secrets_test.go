//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"testing"
)

const stackKey = "dev-only-secret-key-do-not-use-elsewhere"

func rotate(t *testing.T, oldKey, newKey string) (string, error) {
	t.Helper()
	cmd := exec.Command("platform", "secrets", "rotate")
	cmd.Env = []string{"PLATFORM_DATABASE_URL=" + databaseURL, "PLATFORM_OLD_SECRET_KEY=" + oldKey, "PLATFORM_SECRET_KEY=" + newKey}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestSecretsRotateReSealsEverythingAndIsIdempotent(t *testing.T) {
	plainUser.instance(t)
	// Put the stack back on its own key whatever happens, or later tests could not open secrets.
	t.Cleanup(func() {
		if out, err := rotate(t, "rotated-key", stackKey); err != nil {
			t.Fatalf("restore: %v %s", err, out)
		}
	})

	out, err := rotate(t, stackKey, "rotated-key")
	if err != nil || strings.Contains(out, "re-sealed 0 ") {
		t.Fatalf("rotate: %v %q, want some secrets re-sealed", err, out)
	}
	if out, err := rotate(t, stackKey, "rotated-key"); err != nil || out != "re-sealed 0 secrets" {
		t.Fatalf("repeat: %v %q, want nothing left to re-seal", err, out)
	}
	if out, err := rotate(t, "wrong-old-key", "another-key"); err == nil {
		t.Fatalf("a wrong old key was accepted: %q", out)
	}
}
