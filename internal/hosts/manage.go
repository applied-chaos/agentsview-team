package hosts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

type RenameHook func(ctx context.Context, userID string, h store.Host, oldName string)

// installMode is what one mode installs and removes with; update re-runs the install, which
// upgrades in place and keeps the Credential and mode.
type installMode struct{ update, uninstall func(baseURL string) string }

func curlBash(path string) func(string) string {
	return func(base string) string { return "curl -fsSL " + base + path + " | bash" }
}

var installModes = map[store.InstallMode]installMode{
	store.ModeSystemd: {curlBash("/install/linux.sh"), curlBash("/uninstall/linux.sh")},
	store.ModeProfile: {
		func(base string) string { return "curl -fsSL " + base + "/install/linux.sh | bash -s -- --profile" },
		curlBash("/uninstall/linux.sh"),
	},
	store.ModeLaunchd: {curlBash("/install/macos.sh"), curlBash("/install/macos-uninstall.sh")},
	store.ModeTask: {
		func(base string) string { return "irm " + base + "/install/windows.ps1 | iex" },
		func(base string) string { return "irm " + base + "/install/windows-uninstall.ps1 | iex" },
	},
}

// uninstallCommand is what the User runs on the machine; the script detects the install mode.
func (s *Service) uninstallCommand(h store.Host) string {
	return installModes[h.InstallMode].uninstall(s.baseURL)
}

func (s *Service) updateCommand(h store.Host) string {
	return installModes[h.InstallMode].update(s.baseURL)
}

func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	h, err := s.st.HostOf(r.Context(), u.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrHostNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not read Host")
		return
	}
	use, stale := s.cachedUsage(r.Context(), u.ID, "raw-device:"+hex.EncodeToString([]byte(h.DeviceID)))
	httpx.JSON(w, http.StatusOK, map[string]any{
		"host": s.view(h), "pinnedVersion": s.version, "uninstallCommand": s.uninstallCommand(h),
		"updateCommand": s.updateCommand(h), "usage": use, "usageStale": stale,
	})
}

func (s *Service) rename(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	var q struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&q); err != nil || !nameRe.MatchString(q.Name) {
		httpx.Fail(w, http.StatusBadRequest, "use 1 to 64 letters, digits, dots, dashes or underscores")
		return
	}
	ctx := r.Context()
	cur, err := s.st.HostOf(ctx, u.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrHostNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not read Host")
		return
	}
	if !s.auditOnBehalf(w, r, store.ActionImpersonatedHostRename, cur.Name+" to "+q.Name) {
		return
	}
	before, after, err := s.st.RenameHost(ctx, u.ID, r.PathValue("id"), q.Name, func(deviceID string) error {
		return s.dev.RenameDevice(ctx, u.ID, deviceID, q.Name)
	})
	switch {
	case errors.Is(err, store.ErrHostNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, store.ErrHostName):
		httpx.Fail(w, http.StatusConflict, "another Host already has that name, or this Host is revoked")
		return
	case err != nil:
		slog.Error("rename host", "user", u.ID, "err", err)
		httpx.Fail(w, http.StatusServiceUnavailable, "could not rename the Host; try again in a minute")
		return
	}
	if s.OnRename != nil && before.Name != after.Name {
		s.OnRename(ctx, u.ID, after, before.Name)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) revoke(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	cur, err := s.st.HostOf(ctx, u.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrHostNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not read Host")
		return
	}
	if !s.auditOnBehalf(w, r, store.ActionImpersonatedHostRevoke, cur.Name) {
		return
	}
	_, err = s.st.RevokeUserHost(ctx, u.ID, r.PathValue("id"), s.clk.Now(), func(deviceID string) error {
		return s.dev.RevokeDevice(ctx, u.ID, deviceID)
	})
	switch {
	case errors.Is(err, store.ErrHostNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		slog.Error("revoke host", "user", u.ID, "err", err)
		httpx.Fail(w, http.StatusServiceUnavailable, "could not revoke the Host; try again in a minute")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	cur, err := s.st.HostOf(ctx, u.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrHostNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not read Host")
		return
	}
	if !s.auditOnBehalf(w, r, store.ActionImpersonatedHostDelete, cur.Name) {
		return
	}
	err = s.st.DeleteUserHost(ctx, u.ID, cur.ID, func(deviceID string) error {
		return s.dev.RevokeDevice(ctx, u.ID, deviceID)
	})
	switch {
	case errors.Is(err, store.ErrHostNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		slog.Error("delete host", "user", u.ID, "err", err)
		httpx.Fail(w, http.StatusServiceUnavailable, "could not delete the Host; try again in a minute")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// auditOnBehalf records an Impersonation action before it happens and fails the request if it
// cannot, so no action goes unrecorded. It is a no-op for a User acting as themselves.
func (s *Service) auditOnBehalf(w http.ResponseWriter, r *http.Request, action store.Action, details string) bool {
	claims, ok := s.authn.Principal(r)
	if !ok {
		return true
	}
	if err := s.authn.RecordOnBehalf(r.Context(), claims, action, details); err != nil {
		slog.Error("audit impersonated action", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "could not record the action; try again")
		return false
	}
	return true
}

// checkin is the install wrapper's heartbeat: it authenticates with the Host's Credential
// and reports the installed agentsview version, which is how an outdated client is noticed.
func (s *Service) checkin(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&q); err != nil || !versionRe.MatchString(q.Version) {
		httpx.Fail(w, http.StatusBadRequest, "malformed check-in")
		return
	}
	ctx := r.Context()
	deviceID := r.Header.Get("X-AgentsView-Device-ID")
	credential, hasBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	h, err := s.st.HostByDevice(ctx, deviceID)
	if err != nil || !hasBearer || h.RevokedAt != nil {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorised")
		return
	}
	if owner, err := s.st.UserByID(ctx, h.UserID); err != nil || owner.Disabled {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorised")
		return
	}
	if valid, err := s.dev.CredentialValid(ctx, h.UserID, deviceID, credential); err != nil || !valid {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorised")
		return
	}
	if err := s.st.ReportHostVersion(ctx, deviceID, q.Version, s.clk.Now()); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not record check-in")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
