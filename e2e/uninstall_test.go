//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func uninstallCommand(flags string) string {
	cmd := "curl -fsSL " + platformURL + "/uninstall/linux.sh | bash"
	if flags != "" {
		cmd += " -s -- " + flags
	}
	return cmd
}

func uninstallAndVerify(t *testing.T, m *machine, b *browser, flags string) {
	t.Helper()
	l := m.newLogin(t)
	l.mustSh(installCommand(b.newToken(), ""))
	eventually(t, "watcher running before uninstall", 60*time.Second, func() bool {
		out, _ := l.sh(`pgrep -u "$(id -u)" -f '[a]gentsview source raw watch' || true`)
		return out != ""
	})

	l.mustSh(uninstallCommand(flags))
	for _, path := range []string{"~/.local/bin/agentsview", "~/.config/agentsview-platform", "~/.local/share/agentsview-platform/run.sh",
		"~/.config/systemd/user/agentsview-host.service"} {
		if out, _ := l.sh("ls -d " + path + " 2>&1"); !strings.Contains(out, "No such file") {
			t.Fatalf("%s left behind: %s", path, out)
		}
	}
	if h := profileHooks(l); h != "0" {
		t.Fatalf("%s profile hooks left", h)
	}
	if out, _ := l.sh(`ps -u "$(id -u)" -o stat=,args= | awk '$1 !~ /^Z/ && /[a]gentsview/' || true`); out != "" {
		t.Fatalf("processes still running:\n%s", out)
	}
	kept, _ := l.sh("ls -d ~/.local/share/agentsview-platform 2>&1")
	if purged := strings.Contains(kept, "No such file"); purged != (flags == "--purge") {
		t.Fatalf("data dir state %q with flags %q", kept, flags)
	}
	l.mustSh(uninstallCommand(flags))
}

func TestUninstallRemovesAProfileModeInstallAndKeepsDataUnlessPurged(t *testing.T) {
	t.Parallel()
	m := machineFor(t, "linux-profile")
	b, _, _ := running(t, "uninstall")
	uninstallAndVerify(t, m, b, "")
	uninstallAndVerify(t, m, b, "--purge")
}

func TestUninstallRemovesASystemdInstall(t *testing.T) {
	t.Parallel()
	b, _, _ := running(t, "uninstall-systemd")
	uninstallAndVerify(t, machineFor(t, "linux-systemd"), b, "--purge")
}
