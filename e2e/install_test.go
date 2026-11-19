//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type machine struct {
	id      string
	systemd bool
}

func docker(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("docker", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func mustDocker(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	out, err := docker(t, stdin, args...)
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, out)
	}
	return out
}

func machineFor(t *testing.T, service string) *machine {
	t.Helper()
	self, _ := os.Hostname()
	project := mustDocker(t, "", "inspect", "--format", `{{index .Config.Labels "com.docker.compose.project"}}`, self)
	id := mustDocker(t, "", "ps", "-q",
		"--filter", "label=com.docker.compose.project="+project, "--filter", "label=com.docker.compose.service="+service)
	if id == "" {
		t.Fatalf("no %s container in project %s", service, project)
	}
	m := &machine{id: id, systemd: service == "linux-systemd"}
	if m.systemd {
		eventually(t, "systemd booted", 90*time.Second, func() bool {
			out, _ := docker(t, "", "exec", id, "systemctl", "is-system-running")
			return out == "running" || out == "degraded"
		})
	}
	return m
}

type login struct {
	m    *machine
	t    *testing.T
	name string
	uid  string
	// salt keeps session, message and request ids unique across logins: upstream dedupes usage by them.
	salt int
}

var logins atomic.Int64

func (m *machine) newLogin(t *testing.T) *login {
	t.Helper()
	name := fmt.Sprintf("dev%d", time.Now().UnixNano()%1_000_000_000+logins.Add(1))
	mustDocker(t, "", "exec", m.id, "useradd", "-m", "-s", "/bin/bash", name)
	l := &login{m: m, t: t, name: name, uid: mustDocker(t, "", "exec", m.id, "id", "-u", name)}
	if m.systemd {
		mustDocker(t, "", "exec", m.id, "loginctl", "enable-linger", name)
		eventually(t, "user manager up", 60*time.Second, func() bool {
			_, err := l.sh("systemctl --user show-environment")
			return err == nil
		})
	}
	return l
}

func (l *login) sh(script string) (string, error) {
	return docker(l.t, "", "exec", "-u", l.name,
		"-e", "XDG_RUNTIME_DIR=/run/user/"+l.uid, "-e", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+l.uid+"/bus",
		l.m.id, "bash", "-c", script)
}

func (l *login) mustSh(script string) string {
	l.t.Helper()
	out, err := l.sh(script)
	if err != nil {
		l.t.Fatalf("%s: %v\n%s", script, err, out)
	}
	return out
}

func (l *login) writeSessions(n int) { l.writeSessionRange(0, n) }

func (l *login) writeSessionRange(from, to int) {
	l.t.Helper()
	l.mustSh("mkdir -p ~/.claude/projects/-home-" + l.name + "-app")
	for i := from; i < to; i++ {
		id := fmt.Sprintf("0000000%d-aaaa-4bbb-8ccc-%012d", i, l.salt+i+1)
		var lines []string
		prev := "null"
		for turn := 0; turn < 3; turn++ {
			ts := time.Now().Add(clockOffset).UTC().Add(-48*time.Hour + time.Duration(i*60+turn*10)*time.Second).Format("2006-01-02T15:04:05Z")
			u := fmt.Sprintf("%08d-0000-4000-8000-%012d", i*100+turn*2+1, l.salt+i)
			a := fmt.Sprintf("%08d-0000-4000-8000-%012d", i*100+turn*2+2, l.salt+i)
			base := fmt.Sprintf(`"sessionId":%q,"cwd":"/home/%s/app","version":"2.0.30","gitBranch":"main","isSidechain":false,"userType":"external"`, id, l.name)
			lines = append(lines,
				fmt.Sprintf(`{%s,"type":"user","uuid":%q,"parentUuid":%s,"timestamp":%q,"message":{"role":"user","content":"please fix test %d"}}`, base, u, prev, ts, turn),
				fmt.Sprintf(`{%s,"type":"assistant","uuid":%q,"parentUuid":%q,"timestamp":%q,"requestId":"req_%d_%d","message":{"id":"msg_%d_%d","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[{"type":"text","text":"done %d"}],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":300,"cache_read_input_tokens":1000,"cache_creation_input_tokens":100}}}`,
					base, a, u, ts, l.salt+i, turn, l.salt+i, turn, turn))
			prev = fmt.Sprintf("%q", a)
		}
		mustDocker(l.t, strings.Join(lines, "\n")+"\n", "exec", "-i", "-u", l.name, l.m.id, "bash", "-c",
			fmt.Sprintf("cat > ~/.claude/projects/-home-%s-app/%s.jsonl", l.name, id))
	}
}

// writeCodexSession leaves ~/.codex/sessions with one session and no archived_sessions,
// as on a machine where Codex never archived anything.
func (l *login) writeCodexSession() {
	l.t.Helper()
	id := fmt.Sprintf("0199aaaa-bbbb-7ccc-8ddd-%012d", l.salt+1)
	ts := time.Now().Add(clockOffset).UTC().Add(-47 * time.Hour).Format("2006-01-02T15:04:05Z")
	lines := []string{fmt.Sprintf(`{"type":"session_meta","timestamp":%q,"payload":{"id":%q,"cwd":"/home/%s/app","originator":"codex_cli_rs"}}`, ts, id, l.name)}
	// Several turns: the Instance's session count leaves out one-shot sessions.
	for turn := 0; turn < 3; turn++ {
		lines = append(lines,
			fmt.Sprintf(`{"type":"response_item","timestamp":%q,"payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"codex please fix %d"}]}}`, ts, turn),
			fmt.Sprintf(`{"type":"response_item","timestamp":%q,"payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"codex done %d"}]}}`, ts, turn))
	}
	l.mustSh("mkdir -p ~/.codex/sessions/2025/01/01")
	mustDocker(l.t, strings.Join(lines, "\n")+"\n", "exec", "-i", "-u", l.name, l.m.id, "bash", "-c",
		fmt.Sprintf("cat > ~/.codex/sessions/2025/01/01/rollout-2025-01-01T00-00-00-%s.jsonl", id))
}

const backfillMarker = "~/.local/share/agentsview-platform/backfilled"

func installCommand(tok tokenReply, flags string) string {
	cmd := fmt.Sprintf("curl -fsSL %s/install/linux.sh | AVT_TOKEN=%s bash", tok.BaseURL, tok.Token)
	if flags != "" {
		cmd += " -s -- " + flags
	}
	return cmd
}

func sessionCount(b *browser, base string) int {
	s := decode[struct {
		SessionCount int `json:"session_count"`
	}](b.t, b.get(base+"api/v1/stats"))
	return s.SessionCount
}

type overviewReply struct {
	Usage *struct {
		Sessions   int        `json:"sessions"`
		Tokens     int        `json:"tokens"`
		ActiveDays int        `json:"activeDays"`
		Daily      []struct{} `json:"daily"`
	} `json:"usage"`
}

func (b *browser) findHost(name string) *hostView {
	for _, h := range b.hosts().Hosts {
		if h.Name == name {
			return &h
		}
	}
	return nil
}

func installAndVerify(t *testing.T, m *machine, flags, wantMode string, alive func(l *login) string) {
	t.Helper()
	b, _, base := running(t, "install")
	l := m.newLogin(t)
	l.writeSessions(2)
	l.writeCodexSession()
	t.Cleanup(func() {
		if t.Failed() {
			out, _ := l.sh("cat ~/.local/share/agentsview-platform/backfill.log; ls ~/.local/share/agentsview-platform ~/.local/share/agentsview-platform/backfilled.d; ps -u $(id -u) -o pid,ppid,args")
			t.Logf("backfill log:\n%s", out)
		}
	})
	if _, err := l.sh("test -e ~/.codex/archived_sessions"); err == nil {
		t.Fatal("test home must lack ~/.codex/archived_sessions")
	}

	tok := b.newToken()
	out := l.mustSh(installCommand(tok, flags))
	t.Log(out)

	state, host := b.tokenState(tok.ID)
	if state != "connected" || host == nil {
		t.Fatalf("token state %q", state)
	}
	if host.InstallMode != wantMode || host.Status != "online" || host.Name != strings.TrimSpace(mustDocker(t, "", "exec", m.id, "hostname")) {
		t.Fatalf("Host %+v, want mode %s online named after the machine", host, wantMode)
	}
	pinned := b.hosts().PinnedVersion
	if v := l.mustSh("~/.local/bin/agentsview --version"); !strings.Contains(v, "v"+pinned+" ") {
		t.Fatalf("installed %q, want pinned %s", v, pinned)
	}

	eventually(t, "backfilled sessions in the Instance", 180*time.Second, func() bool { return sessionCount(b, base) == 3 })
	eventually(t, "backfill marker", 30*time.Second, func() bool {
		_, err := l.sh("test -s " + backfillMarker)
		return err == nil
	})
	var ov overviewReply
	eventually(t, "Overview figures from the Instance", 60*time.Second, func() bool {
		ov = decode[overviewReply](t, b.get("/api/overview"))
		return ov.Usage != nil && ov.Usage.Sessions == 2
	})
	if u := ov.Usage; u.Tokens <= 0 || u.ActiveDays < 1 || len(u.Daily) == 0 {
		t.Fatalf("usage figures %+v", u)
	}
	eventually(t, "Host detail figures for this Host", 60*time.Second, func() bool {
		d := decode[struct {
			Usage *struct {
				Sessions int `json:"sessions"`
			} `json:"usage"`
		}](t, b.get("/api/hosts/"+host.ID))
		return d.Usage != nil && d.Usage.Sessions == 2
	})
	eventually(t, "last upload recorded", 30*time.Second, func() bool {
		h := b.findHost(host.Name)
		return h != nil && h.LastUploadAt != nil && h.Status == "online"
	})
	if got := alive(l); got == "" {
		t.Fatal("nothing keeps the watcher running")
	}

	// A Host whose backfill never finished has no marker: re-running the install must heal it.
	l.mustSh("rm -f " + backfillMarker)
	before := l.mustSh("cat ~/.config/agentsview-platform/host.env")
	out = l.mustSh(installCommand(tok, flags)) // the token is spent: a re-run must not need it
	t.Log(out)
	if after := l.mustSh("cat ~/.config/agentsview-platform/host.env"); after != before {
		t.Fatal("re-run replaced the Host's Credential")
	}
	if n := len(b.hosts().Hosts); n != 1 {
		t.Fatalf("re-run left %d Hosts, want 1", n)
	}
	eventually(t, "backfill marker rewritten by the re-run", 60*time.Second, func() bool {
		_, err := l.sh("test -s " + backfillMarker)
		return err == nil
	})
	if n := sessionCount(b, base); n != 3 {
		t.Fatalf("re-run changed the session count to %d, want 3", n)
	}
	if got := alive(l); strings.Count(got, "\n") != 0 {
		t.Fatalf("re-run left more than one watcher:\n%s", got)
	}
}

func profileHooks(l *login) string {
	return l.mustSh(`cat ~/.profile ~/.bashrc 2>/dev/null | grep -c '>>> agentsview host >>>' || true`)
}

func TestLinuxInstallWithoutAServiceManagerRunsAWatcherFromShellProfiles(t *testing.T) {
	t.Parallel()
	m := machineFor(t, "linux-profile")
	installAndVerify(t, m, "", "profile", func(l *login) string {
		out, _ := l.sh(`pgrep -u "$(id -u)" -f '[a]gentsview source raw watch' || true`)
		if h := profileHooks(l); h != "2" {
			t.Fatalf("profile hook count %s, want one each in .profile and .bashrc", h)
		}
		eventually(t, "watcher log", 30*time.Second, func() bool {
			log, _ := l.sh("cat ~/.local/share/agentsview-platform/agent.log")
			return strings.Contains(log, "backfill initial-claude: done, 2 sessions") &&
				strings.Contains(log, "initial backfill complete") && strings.Contains(log, "starting watcher")
		})
		return out
	})
}

func TestLinuxInstallUsesASystemdUserService(t *testing.T) {
	t.Parallel()
	m := machineFor(t, "linux-systemd")
	installAndVerify(t, m, "", "systemd", func(l *login) string {
		if out, err := l.sh("systemctl --user is-active agentsview-host.service"); err != nil || out != "active" {
			t.Fatalf("service %q %v", out, err)
		}
		if out := l.mustSh("systemctl --user is-enabled agentsview-host.service"); out != "enabled" {
			t.Fatalf("service not enabled: %s", out)
		}
		if h := profileHooks(l); h != "0" {
			t.Fatalf("systemd mode left %s profile hooks", h)
		}
		out, _ := l.sh(`pgrep -u "$(id -u)" -f '[a]gentsview source raw watch' || true`)
		return out
	})
}

func TestProfileFlagForcesProfileModeEvenWhereSystemdWorks(t *testing.T) {
	t.Parallel()
	m := machineFor(t, "linux-systemd")
	b, _, _ := running(t, "forced")
	l := m.newLogin(t)
	tok := b.newToken()
	l.mustSh(installCommand(tok, "--profile"))
	if _, host := b.tokenState(tok.ID); host == nil || host.InstallMode != "profile" {
		t.Fatalf("Host %+v, want profile mode", host)
	}
	if out, _ := l.sh("systemctl --user cat agentsview-host.service"); strings.Contains(out, "ExecStart") {
		t.Fatal("a systemd unit was installed in forced profile mode")
	}
	if h := profileHooks(l); h != "2" {
		t.Fatalf("profile hooks %s, want 2", h)
	}
}

func TestInstallFailsClearlyWithAnUnknownOrSpentToken(t *testing.T) {
	t.Parallel()
	m := machineFor(t, "linux-profile")
	l := m.newLogin(t)
	out, err := l.sh("curl -fsSL " + platformURL + "/install/linux.sh | AVT_TOKEN=avt_tk_nope bash")
	if err == nil || !strings.Contains(out, "Could not add this Host (HTTP 401)") {
		t.Fatalf("got %v: %s", err, out)
	}
	if _, err := l.sh("test -e ~/.config/agentsview-platform/host.env"); err == nil {
		t.Fatal("a failed install left a Host config behind")
	}
}
