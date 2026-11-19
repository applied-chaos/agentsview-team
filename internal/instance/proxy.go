package instance

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/auth"
	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

//go:embed bar/bar.css bar/bar.js
var barFiles embed.FS

// Authn is the Identity module as the proxy needs it.
type Authn interface {
	Principal(r *http.Request) (auth.AccessClaims, bool)
	Admin(r *http.Request) (auth.AccessClaims, store.User, error)
	RecordOnBehalf(ctx context.Context, p auth.AccessClaims, action store.Action, details string) error
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const (
	headInject = `<link rel="stylesheet" href="/_platform/bar.css"><script src="/_platform/bar.js" defer></script>`
)

const (
	eyeIcon    = `<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><path d="M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0"/>`
	shieldIcon = `<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>`
)

type bannerKind string

const (
	bannerNone bannerKind = ""
	bannerView bannerKind = "view"
	bannerImp  bannerKind = "imp"
)

// banner is the persistent Instance View or Impersonation notice at the top of the bar; zero for the owner.
type banner struct {
	kind   bannerKind
	target store.User
}

// Tee sees raw-sync traffic to a User's Instance. Intercept may wrap r.Body; the returned
// Responder (nil for none) sees the Instance's response and an error fails it.
type Tee interface {
	Intercept(userID string, r *http.Request) Responder
}

type Responder interface {
	Respond(*http.Response) error
}

func (b banner) html() string {
	if b.kind == bannerNone {
		return ""
	}
	name, id := html.EscapeString(b.target.Name), html.EscapeString(b.target.ID)
	svg := func(paths string) string {
		return `<svg class="i" aria-hidden="true" viewBox="0 0 24 24">` + paths + `</svg>`
	}
	if b.kind == bannerView {
		return fmt.Sprintf(`<div class="banner view" role="status">%s<span>Instance View: you are looking at %s’s Instance as yourself.</span>`+
			`<a class="btn" href="/admin/users/%s">Exit Instance View</a></div>`, svg(eyeIcon), name, id)
	}
	return fmt.Sprintf(`<div class="banner imp" role="status">%s<span>Impersonating %s. Everything you do is recorded in the Audit Log.</span>`+
		`<button class="btn" type="button" data-act="stop-imp" data-user="%s">Stop impersonating</button></div>`, svg(shieldIcon), name, id)
}

func (b banner) bar() string {
	return `<div id="av-platbar">` + b.html() + `<div class="wrap"><a class="back" href="/">` +
		`<svg class="i" aria-hidden="true" viewBox="0 0 24 24"><path d="m15 18-6-6 6-6"/></svg>Back to AgentsView Teams</a></div></div>`
}

// Observer is told which Host a raw-sync request came from.
type Observer interface {
	Seen(ctx context.Context, deviceID string)
	Uploaded(ctx context.Context, deviceID string)
}

// Routes registers the Instance proxy, the Instance status API and the bar assets.
// spa renders the in-Platform page for an Instance that is not running.
func (m *Manager) Routes(mux *http.ServeMux, a Authn, obs Observer, spa http.Handler, webDir string) {
	p := &proxy{m: m, authn: a, spa: spa, obs: obs}
	mux.HandleFunc("GET /api/instance", p.status)
	mux.HandleFunc("POST /api/instance/retry", p.retry)
	mux.Handle("/i/", p)
	m.testRoutes(mux)
	mux.HandleFunc("GET /_platform/bar.js", serveEmbedded("bar/bar.js", "text/javascript; charset=utf-8", nil))
	mux.HandleFunc("GET /_platform/bar.css", serveEmbedded("bar/bar.css", "text/css; charset=utf-8", func() []byte { return fontFaces(webDir) }))
}

func serveEmbedded(name, ctype string, extra func() []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		data, _ := barFiles.ReadFile(name)
		if extra != nil {
			data = append(extra(), data...)
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
	}
}

// fontFaces points the bar at the SPA's self-hosted fonts so it matches Platform pages.
func fontFaces(webDir string) []byte {
	var b bytes.Buffer
	for _, w := range []string{"400", "600", "700"} {
		files, _ := filepath.Glob(filepath.Join(webDir, "assets", "source-sans-3-latin-"+w+"-normal-*.woff2"))
		if len(files) > 0 {
			b.WriteString("@font-face{font-family:'Source Sans 3';font-weight:" + w + ";font-display:swap;src:url('/assets/" +
				filepath.Base(files[0]) + "') format('woff2')}\n")
		}
	}
	return b.Bytes()
}

type proxy struct {
	m     *Manager
	authn Authn
	spa   http.Handler
	obs   Observer
}

// targetID is the Instance a status or retry call is about: the caller's own, or with ?user= an
// Admin's Instance View of another User's.
func (p *proxy) targetID(w http.ResponseWriter, r *http.Request, claims auth.AccessClaims) (string, bool) {
	id := claims.Effective()
	if v := r.URL.Query().Get("user"); v != "" && v != id {
		if _, _, err := p.authn.Admin(r); err != nil || claims.Imp != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return "", false
		}
		id = v
	}
	return id, true
}

func (p *proxy) status(w http.ResponseWriter, r *http.Request) {
	claims, ok := p.authn.Principal(r)
	if !ok {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	id, ok := p.targetID(w, r, claims)
	if !ok {
		return
	}
	u, err := p.m.store.UserByID(r.Context(), id)
	if err != nil || (u.Disabled && id == claims.Effective()) {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	if !u.Disabled {
		p.m.Ensure(u.ID)
	}
	state, _, _ := p.m.Status(u.ID)
	// The detail (host, database and role names) is in the log, not for Users.
	code := ""
	if state == Failed {
		code = "instance_failed"
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"state": string(state), "url": "/i/" + u.ID + "/", "version": PinnedVersion, "error": code})
}

func (p *proxy) retry(w http.ResponseWriter, r *http.Request) {
	claims, ok := p.authn.Principal(r)
	if !ok {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	id, ok := p.targetID(w, r, claims)
	if !ok {
		return
	}
	if err := p.authn.RecordOnBehalf(r.Context(), claims, store.ActionImpersonatedRetry, ""); err != nil {
		slog.Error("audit impersonated retry", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.m.Retry(id)
	w.WriteHeader(http.StatusAccepted)
}

func wantsHTML(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, tail, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/i/"), "/")
	if id == "me" {
		claims, ok := p.authn.Principal(r)
		if !ok {
			p.unauthorised(w, r)
			return
		}
		to := "/i/" + claims.Effective() + "/" + tail
		if r.URL.RawQuery != "" {
			to += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, to, http.StatusFound)
		return
	}
	if !uuidRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	// Host authentication is upstream's; the Platform only checks the User is not Disabled.
	rawSync := strings.HasPrefix(tail, "api/v1/raw-sync/")
	var bn banner
	if rawSync {
		u, err := p.m.store.UserByID(r.Context(), id)
		switch {
		case err != nil:
			http.NotFound(w, r)
			return
		case u.Disabled:
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	} else {
		claims, ok := p.authn.Principal(r)
		if !ok {
			p.unauthorised(w, r)
			return
		}
		var err error
		if bn, err = p.authorise(r, claims, id); err != nil {
			if errors.Is(err, errNoSuchUser) {
				http.NotFound(w, r)
				return
			}
			auth.Reject(w, err)
			return
		}
		// A write made while impersonating is recorded before it reaches the Instance, or refused.
		if bn.kind == bannerImp && !safeMethod(r.Method) {
			if err := p.authn.RecordOnBehalf(r.Context(), claims, store.ActionImpersonatedWrite, r.Method+" /"+tail); err != nil {
				slog.Error("audit impersonated write", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
	}

	target, token, ok := p.m.Target(id)
	if !ok {
		if wantsHTML(r) {
			p.spa.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Retry-After", "5")
		http.Error(w, "instance not running", http.StatusServiceUnavailable)
		return
	}
	var onResp func(*http.Response) error
	if rawSync {
		var tee Responder
		if p.m.Tee != nil {
			tee = p.m.Tee.Intercept(id, r)
		}
		onResp = func(resp *http.Response) error {
			if p.obs != nil {
				p.observe(id, resp)
			}
			if tee != nil {
				return tee.Respond(resp)
			}
			return nil
		}
	}
	reverseProxy(target, token, rawSync, id, bn, onResp).ServeHTTP(w, r)
}

type rawSyncCall int

const (
	callTokens rawSyncCall = iota + 1
	callManifests
)

// observe attributes a successful raw-sync call to its Host. Idle Hosts send nothing, so
// the installed service pings with server-status (see the install script).
func (p *proxy) observe(userID string, resp *http.Response) {
	if resp.StatusCode != http.StatusOK || resp.Request.Method != http.MethodPost {
		return
	}
	var kind rawSyncCall
	switch {
	case strings.HasSuffix(resp.Request.URL.Path, "/raw-sync/tokens"):
		kind = callTokens
	case strings.HasSuffix(resp.Request.URL.Path, "/raw-sync/manifests"):
		kind = callManifests
	default:
		return
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	if err != nil {
		return
	}
	var out struct {
		DeviceID string `json:"device_id"`
		Created  bool   `json:"created"`
	}
	if json.Unmarshal(data, &out) != nil {
		return
	}
	bearer := strings.TrimPrefix(resp.Request.Header.Get("Authorization"), "Bearer ")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if kind == callTokens {
			p.obs.Seen(ctx, out.DeviceID)
			return
		}
		if !out.Created {
			return
		}
		device, err := p.m.DeviceForToken(ctx, userID, bearer)
		if err != nil || device == "" {
			slog.Warn("upload not attributed to a Host", "user", userID, "err", err)
			return
		}
		p.obs.Uploaded(ctx, device)
	}()
}

// reverseProxy forwards to one Instance. Raw-sync requests carry the Host's own
// credentials and are passed untouched; everything else gets the Instance bearer.
func reverseProxy(target *url.URL, token string, rawSync bool, id string, bn banner, onResp func(*http.Response) error) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// Without X-Forwarded-For agentsview treats the request as local and
			// GET /api/v1/settings returns the Instance token.
			pr.SetXForwarded()
			pr.Out.Header.Del("Cookie")
			if !rawSync {
				pr.Out.Header.Set("Authorization", "Bearer "+token)
				pr.Out.Header.Del("Accept-Encoding") // the bar is injected into plain HTML
			}
		},
		FlushInterval: -1, // SSE
		ModifyResponse: func(resp *http.Response) error {
			if onResp != nil {
				if err := onResp(resp); err != nil {
					return err
				}
			}
			return injectBar(resp, bn.bar())
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			slog.Warn("instance proxy", "user", id, "err", err)
			http.Error(w, "instance unavailable", http.StatusBadGateway)
		},
	}
}

var errNoSuchUser = errors.New("no such user")

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// authorise decides who may reach Instance id: its owner, an Admin in Instance View (not audited),
// or an Admin impersonating that owner. Admin is re-checked from the stored role on every request.
func (p *proxy) authorise(r *http.Request, claims auth.AccessClaims, id string) (banner, error) {
	if id == p.m.adminID {
		// Admins only, as themselves: Impersonation must not reach everyone's sessions.
		if _, _, err := p.authn.Admin(r); err != nil {
			return banner{}, err
		}
		if claims.Imp != "" {
			return banner{}, auth.ErrForbidden
		}
		return banner{}, nil
	}
	if claims.Imp == "" && claims.Subject == id {
		return banner{}, nil
	}
	if _, _, err := p.authn.Admin(r); err != nil {
		return banner{}, err
	}
	if claims.Imp != "" && claims.Imp != id {
		return banner{}, auth.ErrForbidden
	}
	target, err := p.m.store.UserByID(r.Context(), id)
	if err != nil {
		return banner{}, errNoSuchUser
	}
	if claims.Imp != "" {
		return banner{kind: bannerImp, target: target}, nil
	}
	return banner{kind: bannerView, target: target}, nil
}

func (p *proxy) unauthorised(w http.ResponseWriter, r *http.Request) {
	if wantsHTML(r) {
		http.Redirect(w, r, "/sign-in", http.StatusFound)
		return
	}
	http.Error(w, "unauthorised", http.StatusUnauthorized)
}

func injectBar(resp *http.Response, bar string) error {
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}
	page, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	page = bytes.Replace(page, []byte("</head>"), []byte(headInject+"</head>"), 1)
	if i := bytes.Index(page, []byte("<body")); i >= 0 {
		if j := bytes.IndexByte(page[i:], '>'); j >= 0 {
			at := i + j + 1
			page = append(page[:at:at], append([]byte(bar), page[at:]...)...)
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(page))
	resp.ContentLength = int64(len(page))
	resp.Header.Set("Content-Length", strconv.Itoa(len(page)))
	return nil
}
