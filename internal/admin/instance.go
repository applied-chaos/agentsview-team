package admin

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/stats"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

func (s *Service) instanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/instance", s.admin(s.adminInstance))
	mux.HandleFunc("GET /api/admin/instance/open", s.admin(s.openAdminInstance))
}

func (s *Service) adminInstanceURL() string { return "/i/" + s.Instances.AdminID() + "/" }

func (s *Service) adminInstance(w http.ResponseWriter, r *http.Request, _ store.Actor) {
	if s.Instances == nil {
		http.NotFound(w, r)
		return
	}
	state, lastErr, _ := s.Instances.Status(s.Instances.AdminID())
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": s.Instances.AdminID(), "state": state, "url": s.adminInstanceURL(), "error": lastErr, "users": s.usageByUser(r), "space": s.space(r),
	})
}

func (s *Service) openAdminInstance(w http.ResponseWriter, r *http.Request, who store.Actor) {
	if s.Instances == nil {
		http.NotFound(w, r)
		return
	}
	err := s.store.RecordAudit(r.Context(), s.clock.Now(), store.AuditEntry{
		Actor: who, Action: store.ActionAdminInstance, TargetLabel: "Admin Instance",
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, s.adminInstanceURL(), http.StatusFound)
}

type userRow struct {
	stats.UserUsage
	StorageBytes int64 `json:"storageBytes"`
}

func (s *Service) space(r *http.Request) stats.Space {
	if s.Stats != nil {
		if sp, err := s.Stats.Space(r.Context()); err == nil {
			return *sp
		}
	}
	return stats.Space{Users: map[string]int64{}}
}

func (s *Service) usageByUser(r *http.Request) []userRow {
	out := []userRow{}
	if s.Stats == nil {
		return out
	}
	m, err := s.Stats.Users(r.Context())
	if err != nil {
		return out
	}
	sp := s.space(r)
	for id, u := range m {
		out = append(out, userRow{u, sp.Users[id]})
	}
	slices.SortFunc(out, func(a, b userRow) int {
		return cmp.Or(cmp.Compare(b.Sessions, a.Sessions), strings.Compare(a.Name, b.Name))
	})
	return out
}
