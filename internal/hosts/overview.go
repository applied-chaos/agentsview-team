package hosts

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/usage"
)

// Instances is the Instance supervisor as the Overview needs it.
type Instances interface {
	Target(userID string) (base *url.URL, token string, ok bool)
}

const (
	usagePageWait = 3 * time.Second
	// usageLoadWithin bounds a background read; a busy Instance can take far longer than a page should wait.
	usageLoadWithin = 60 * time.Second
	usageTTL        = 10 * time.Second
)

type usageDay struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens"`
}

type usageJSON struct {
	From       string     `json:"from"`
	To         string     `json:"to"`
	Sessions   int64      `json:"sessions"`
	Tokens     int64      `json:"tokens"`
	CostUSD    float64    `json:"costUsd"`
	ActiveDays int        `json:"activeDays"`
	Daily      []usageDay `json:"daily"`
}

type usageKey struct{ user, machine string }

type usageEntry struct {
	val     *usageJSON
	at      time.Time
	loading chan struct{} // non-nil while a read is under way; closed when it ends
}

// usageCache keeps the last usage read from each Instance. Reads run in the background, so a
// slow or stopped Instance never holds a page up and a slow first read still lands for the next load.
type usageCache struct {
	mu      sync.Mutex
	entries map[usageKey]usageEntry
}

// cachedUsage returns the last usage; stale means a newer read is under way and worth asking for
// again shortly. With nothing cached it waits up to usagePageWait for the read.
func (s *Service) cachedUsage(ctx context.Context, userID, machine string) (val *usageJSON, stale bool) {
	k := usageKey{userID, machine}
	c := &s.usage
	c.mu.Lock()
	e := c.entries[k]
	if e.val != nil && s.clk.Now().Sub(e.at) < usageTTL {
		c.mu.Unlock()
		return e.val, false
	}
	if e.loading == nil {
		e.loading = make(chan struct{})
		c.entries[k] = e
		go s.loadUsage(k, e.loading)
	}
	c.mu.Unlock()
	if e.val != nil {
		return e.val, true
	}
	select {
	case <-e.loading:
	case <-time.After(usagePageWait):
	case <-ctx.Done():
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e = c.entries[k]
	return e.val, e.loading != nil
}

func (s *Service) loadUsage(k usageKey, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), usageLoadWithin)
	defer cancel()
	v := s.fetchUsage(ctx, k.user, k.machine)
	c := &s.usage
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[k]
	e.loading = nil
	if v != nil {
		e.val, e.at = v, s.clk.Now()
	}
	c.entries[k] = e
	close(done)
}

// fetchUsage reads the User's own last 30 days from their Instance (UTC days), optionally
// for one machine selector (raw-device:<hex>). It returns nil when the Instance is not up or fails.
func (s *Service) fetchUsage(ctx context.Context, userID, machine string) *usageJSON {
	base, token, ok := s.instances.Target(userID)
	if !ok {
		return nil
	}
	to := s.clk.Now().UTC()
	from := to.AddDate(0, 0, -29)
	extra := url.Values{}
	if machine != "" {
		extra.Set("machine", machine)
	}
	sum, err := usage.For(userID, base, token).Summary(ctx, from.Format(time.DateOnly), to.Format(time.DateOnly), extra)
	if err != nil {
		return nil
	}
	out := &usageJSON{
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Sessions: sum.SessionCounts.Total,
		Tokens: sum.Totals.All(), CostUSD: sum.Totals.TotalCost.USD(), Daily: []usageDay{},
	}
	for _, d := range sum.Daily {
		out.Daily = append(out.Daily, usageDay{Date: d.Date, Tokens: d.All()})
		if d.All() > 0 {
			out.ActiveDays++
		}
	}
	return out
}

func (s *Service) overview(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r)
	if !ok {
		return
	}
	hosts, err := s.hostList(r.Context(), u.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "could not list Hosts")
		return
	}
	var space *int64
	if s.Space != nil {
		if n, err := s.Space(r.Context(), u.ID); err == nil {
			space = &n
		}
	}
	use, stale := s.cachedUsage(r.Context(), u.ID, "")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"hosts": hosts, "pinnedVersion": s.version, "usage": use, "usageStale": stale,
		"storageBytes": space,
	})
}
