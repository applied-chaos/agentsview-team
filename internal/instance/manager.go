// Package instance runs each User's agentsview Instance as a supervised child
// process and proxies it under /i/<user-id>/.
package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/config"
	"github.com/applied-chaos/agentsview-team/internal/secrets"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

// State is what the Platform knows of an Instance. Failed means the supervisor
// gave up until someone retries.
type State string

const (
	Starting State = "starting"
	Running  State = "running"
	Failed   State = "failed"
)

const (
	maxFailures  = 3
	healthWithin = 90 * time.Second
)

var backoff = []time.Duration{time.Second, 2 * time.Second}

type Manager struct {
	store                    *store.Store
	box                      *secrets.Box
	bin                      string
	dataDir                  string
	baseURL                  string
	pgBase                   *url.URL
	adminUser, adminPassword string

	// Tee, when set, sees every raw-sync request to a User's Instance (ADR 0003).
	Tee     Tee
	adminID string

	ctx context.Context
	wg  sync.WaitGroup
	mu  sync.Mutex
	set map[string]*handle
}

type handle struct {
	userID string
	retry  chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu    sync.Mutex
	state State
	port  int
	token string
	cmd   *exec.Cmd
	err   string
}

func (h *handle) snapshot() (State, int, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state, h.port, h.token
}

func (h *handle) setErr(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.err = ""
	if err != nil {
		h.err = err.Error()
	}
}

func (h *handle) set(s State, port int, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state, h.port, h.token = s, port, token
}

// PinnedVersion is the agentsview release the grants in provision.go were checked against.
// Dockerfile sets it at build time (-ldflags -X) from the same ARG as the base image tag.
var PinnedVersion string

// pinnedSchemaVersion is the `schema_version` of `agentsview version --json` that the pinned
// release reports; a different value means the output contract this code reads has changed.
const pinnedSchemaVersion = 1

// CheckBinary refuses a binary that is not the pinned release: Instances would
// be provisioned and upgraded by code the grants were never checked against.
func CheckBinary(ctx context.Context, bin string) error {
	out, err := exec.CommandContext(ctx, bin, "version", "--json").Output()
	if err != nil {
		return fmt.Errorf("agentsview version: %w", err)
	}
	return checkVersion(out)
}

func checkVersion(out []byte) error {
	var v struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return fmt.Errorf("agentsview version output: %w", err)
	}
	if v.SchemaVersion != pinnedSchemaVersion {
		return fmt.Errorf("agentsview reports schema version %d, Platform is pinned to %d", v.SchemaVersion, pinnedSchemaVersion)
	}
	if got := strings.TrimPrefix(v.Version, "v"); got != PinnedVersion {
		return fmt.Errorf("agentsview binary is %q, Platform is pinned to %s", v.Version, PinnedVersion)
	}
	return nil
}

// New prepares a Manager. The database URL's credentials are the `platform`
// role (CREATEDB, CREATEROLE, no superuser); its host and options are reused
// for every Instance login.
func New(ctx context.Context, cfg config.Config, st *store.Store, box *secrets.Box) (*Manager, error) {
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	pw, _ := u.User.Password()
	if err := CheckBinary(ctx, cfg.AgentsviewBin); err != nil {
		return nil, err
	}
	m := &Manager{
		store: st, box: box, bin: cfg.AgentsviewBin, dataDir: cfg.DataDir, baseURL: cfg.BaseURL,
		pgBase: u, adminUser: u.User.Username(), adminPassword: pw,
		ctx: ctx, set: map[string]*handle{},
	}
	// The parser materialises files under TMPDIR; the root filesystem is read-only.
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "tmp"), 0o700); err != nil {
		return nil, err
	}
	if err := m.initAdmin(ctx); err != nil {
		return nil, err
	}
	m.Ensure(m.adminID)
	ids, err := st.EnabledInstanceUserIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		m.Ensure(id)
	}
	return m, nil
}

func (m *Manager) AdminID() string { return m.adminID }

// Wait blocks until every supervisor has stopped its child; cancel the context first.
func (m *Manager) Wait() { m.wg.Wait() }

// Ensure starts supervising the User's Instance, provisioning it first. Safe to repeat.
func (m *Manager) Ensure(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.set[userID]; ok || m.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	h := &handle{userID: userID, state: Starting, retry: make(chan struct{}, 1), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	m.set[userID] = h
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer close(h.done)
		m.supervise(h)
	}()
}

// Stop ends the User's Instance and waits for its process to exit. Safe to repeat;
// Ensure starts it again.
func (m *Manager) Stop(userID string) {
	m.mu.Lock()
	h := m.set[userID]
	delete(m.set, userID)
	m.mu.Unlock()
	if h != nil {
		h.cancel()
		<-h.done
	}
}

func (m *Manager) handle(userID string) *handle {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.set[userID]
}

// State reports the Instance's state; ok is false when nothing supervises it.
func (m *Manager) State(userID string) (State, bool) {
	s, _, ok := m.Status(userID)
	return s, ok
}

// Status is State plus why the last start failed (empty once running).
func (m *Manager) Status(userID string) (s State, lastErr string, ok bool) {
	h := m.handle(userID)
	if h == nil {
		return "", "", false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state, h.err, true
}

// Retry restarts an Instance the supervisor gave up on.
func (m *Manager) Retry(userID string) {
	if h := m.handle(userID); h != nil {
		select {
		case h.retry <- struct{}{}:
		default:
		}
	}
}

// Target is where a running Instance listens and the bearer token it expects.
func (m *Manager) Target(userID string) (base *url.URL, token string, ok bool) {
	h := m.handle(userID)
	if h == nil {
		return nil, "", false
	}
	s, port, tok := h.snapshot()
	if s != Running {
		return nil, "", false
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", fmt.Sprint(port))}, tok, true
}

// supervise provisions then runs the child, restarting it with backoff and
// giving up after maxFailures consecutive failures to reach health.
func (m *Manager) supervise(h *handle) {
	failures := 0
	for h.ctx.Err() == nil {
		healthy, err := m.runOnce(h)
		h.set(Starting, 0, "")
		if h.ctx.Err() != nil {
			return
		}
		if healthy {
			failures = 0
		}
		failures++
		slog.Warn("instance stopped", "user", h.userID, "failures", failures, "err", err)
		h.setErr(err)
		if failures >= maxFailures {
			h.set(Failed, 0, "")
			select {
			case <-h.retry:
				failures = 0
				continue
			case <-h.ctx.Done():
				return
			}
		}
		select {
		case <-time.After(backoff[min(failures-1, len(backoff)-1)]):
		case <-h.retry:
		case <-h.ctx.Done():
			return
		}
	}
}

// runOnce provisions, starts the child and blocks until it exits.
func (m *Manager) runOnce(h *handle) (healthy bool, err error) {
	if err := beforeStart(h.userID); err != nil {
		return false, err
	}
	// Re-running hosted-provision is the upgrade: it migrates the schema or refuses (wrong tenant, newer data).
	sec, dir, err := m.provision(h.ctx, h.userID)
	if err != nil {
		return false, fmt.Errorf("agentsview upgrade to %s: %w", PinnedVersion, err)
	}
	port, err := freePort(h.ctx)
	if err != nil {
		return false, err
	}
	base := "/i/" + h.userID
	cmd := exec.CommandContext(h.ctx, m.bin, "replica", "pg", "serve",
		"--host", "127.0.0.1", "--port", fmt.Sprint(port), "--no-browser",
		"--base-path", base, "--public-url", m.baseURL, "--no-update-check")
	cmd.Env = m.childEnv(dir, sec.token)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	logf, err := os.OpenFile(filepath.Join(dir, "serve.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer func() { _ = logf.Close() }()
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return false, err
	}
	h.mu.Lock()
	h.cmd = cmd
	h.mu.Unlock()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	ping := time.NewTicker(250 * time.Millisecond)
	defer ping.Stop()
	deadline := time.After(healthWithin)
	for !healthy {
		select {
		case err := <-exited:
			return false, fmt.Errorf("exited before healthy: %w", err)
		case <-deadline:
			_ = cmd.Process.Kill()
			slog.Debug("unhealthy instance reaped", "user", h.userID, "err", <-exited)
			return false, fmt.Errorf("not healthy within %s", healthWithin)
		case <-ping.C:
			healthy = pingOK(h.ctx, port, base, sec.token)
		}
	}
	h.set(Running, port, sec.token)
	h.setErr(nil)
	return true, <-exited
}

func (m *Manager) childEnv(dir, token string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + filepath.Join(m.dataDir, "tmp"),
		"AGENTSVIEW_DATA_DIR=" + dir,
		"AGENTSVIEW_TELEMETRY_ENABLED=0",
	}
	if token != "" {
		env = append(env, "AGENTSVIEW_AUTH_TOKEN="+token)
	}
	return env
}

func pingOK(ctx context.Context, port int, base, token string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s/api/ping", port, base), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

func freePort(ctx context.Context) (int, error) {
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
