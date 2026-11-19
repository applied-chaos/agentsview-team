// Package stats computes the Team page and the Admin per-User breakdown from the Admin
// Instance's usage API (grouping devices on their "<user email>/" label prefix) and from a
// User's own Instance for their own figures. A background refresher recomputes them every five
// minutes; requests only read the cache and never wait on an upstream.
package stats

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/auth"
	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/httpx"
	"github.com/applied-chaos/agentsview-team/internal/store"
	"github.com/applied-chaos/agentsview-team/internal/usage"
)

const (
	// RefreshEvery is how often the refresher recomputes everything.
	RefreshEvery = 5 * time.Minute
	retryEvery   = 30 * time.Second
	kickEvery    = time.Minute
	days         = 30
	topN         = 5
)

type Instances interface {
	Target(userID string) (base *url.URL, token string, ok bool)
	AdminID() string
}

type Service struct {
	st      *store.Store
	clk     *clock.Clock
	authn   *auth.Service
	inst    Instances
	dataDir string

	// mu guards the caches only; it is never held over an upstream call.
	mu        sync.Mutex
	orgCache  *orgData
	own       map[string]*Figures
	space     *Space
	lastKick  map[string]time.Time
	inflight  map[string]bool
	refreshMu sync.Mutex
}

func New(st *store.Store, clk *clock.Clock, a *auth.Service, inst Instances, dataDir string) *Service {
	return &Service{st: st, clk: clk, authn: a, inst: inst, dataDir: dataDir, own: map[string]*Figures{}, lastKick: map[string]time.Time{}, inflight: map[string]bool{}}
}

func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/team", s.team)
}

type Day struct {
	Date        string  `json:"date"`
	Sessions    int64   `json:"sessions"`
	Tokens      int64   `json:"tokens"`
	Cost        float64 `json:"cost"`
	ActiveUsers int     `json:"activeUsers"`
	ActiveHosts int     `json:"activeHosts"`
}

type Ranked struct {
	Name     string `json:"name"`
	Sessions int64  `json:"sessions"`
	Tokens   int64  `json:"tokens"`
	Share    int    `json:"share"`
}

// Figures are one scope's last-30-days numbers.
type Figures struct {
	Sessions    int64    `json:"sessions"`
	TokensIn    int64    `json:"tokensIn"`
	TokensOut   int64    `json:"tokensOut"`
	CostUSD     float64  `json:"costUsd"`
	ActiveUsers int      `json:"activeUsers"`
	ActiveHosts int      `json:"activeHosts"`
	Daily       []Day    `json:"daily"`
	Models      []Ranked `json:"models"`
	Agents      []Ranked `json:"agents"`
}

// UserUsage is one User's row in the Admin breakdown.
type UserUsage struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Sessions   int64      `json:"sessions"`
	Tokens     int64      `json:"tokens"`
	Cost       float64    `json:"cost"`
	Hosts      int        `json:"hosts"`
	LastActive *time.Time `json:"lastActive"`
	Daily      []Day      `json:"-"`
}

type orgData struct {
	Org   *Figures
	Users map[string]UserUsage // by User ID
}

func (s *Service) window() (from, to string) {
	now := s.clk.Now().UTC()
	return now.AddDate(0, 0, -(days - 1)).Format(time.DateOnly), now.Format(time.DateOnly)
}

func (s *Service) source(id string) (usage.Source, bool) {
	base, token, ok := s.inst.Target(id)
	return usage.For(id, base, token), ok
}

func figures(sum usage.Summary) *Figures {
	t := sum.Totals
	f := &Figures{
		Sessions: sum.SessionCounts.Total, TokensOut: t.OutputTokens,
		TokensIn: t.InputTokens + t.CacheCreationTokens + t.CacheReadTokens,
		CostUSD:  t.TotalCost.USD(), Daily: []Day{}, Models: []Ranked{}, Agents: []Ranked{},
	}
	for _, d := range sum.Daily {
		f.Daily = append(f.Daily, Day{Date: d.Date, Tokens: d.All(), Cost: d.TotalCost.USD()})
	}
	return f
}

func share(n, total int64) int {
	if total == 0 {
		return 0
	}
	return int((n*100 + total/2) / total)
}

// padDays returns exactly one entry per day in the window, zero for quiet days.
func padDays(have []Day, from string) []Day {
	byDate := map[string]Day{}
	for _, d := range have {
		byDate[d.Date] = d
	}
	start, _ := time.Parse(time.DateOnly, from)
	out := make([]Day, days)
	for i := range out {
		date := start.AddDate(0, 0, i).Format(time.DateOnly)
		d := byDate[date]
		d.Date = date
		out[i] = d
	}
	return out
}

// forEach runs fn on every item, a few at a time, and returns the joined errors.
func forEach[T any](items []T, fn func(T) error) error {
	const parallel = 4
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, parallel)
	)
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if err := fn(it); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

type userLoad struct {
	row   UserUsage
	days  map[string]bool   // days the User had sessions
	hosts []map[string]bool // per device: days with sessions
}

func (s *Service) loadOrg(ctx context.Context) (*orgData, error) {
	src, ok := s.source(s.inst.AdminID())
	if !ok {
		return nil, errors.New("the Admin Instance is not running")
	}
	from, to := s.window()
	sum, err := src.Summary(ctx, from, to, nil)
	if err != nil {
		return nil, err
	}
	org := figures(sum)
	orgSessions, err := src.SessionDays(ctx, from, to, "")
	if err != nil {
		return nil, err
	}
	for i := range org.Daily {
		org.Daily[i].Sessions = orgSessions[org.Daily[i].Date]
	}
	total := org.TokensIn + org.TokensOut
	for _, m := range sum.ModelTotals {
		org.Models = append(org.Models, Ranked{Name: m.Model, Tokens: m.All()})
	}
	for _, a := range sum.AgentTotals {
		org.Agents = append(org.Agents, Ranked{Name: a.Agent, Sessions: sum.SessionCounts.ByAgent[a.Agent], Tokens: a.All()})
	}
	for _, list := range []*[]Ranked{&org.Models, &org.Agents} {
		slices.SortFunc(*list, func(a, b Ranked) int { return cmp.Compare(b.Tokens, a.Tokens) })
		*list = (*list)[:min(len(*list), topN)]
		for i := range *list {
			(*list)[i].Share = share((*list)[i].Tokens, total)
		}
	}
	err = forEach(idx2(len(org.Models)), func(i int) error {
		ms, err := src.Summary(ctx, from, to, url.Values{"model": {org.Models[i].Name}})
		org.Models[i].Sessions = ms.SessionCounts.Total
		return err
	})
	if err != nil {
		return nil, err
	}

	var machines struct {
		Labels map[string]string `json:"machine_labels"`
	}
	if err := src.Get(ctx, "machines", url.Values{}, &machines); err != nil {
		return nil, err
	}
	users, err := s.st.StatsUsers(ctx)
	if err != nil {
		return nil, err
	}
	loads := make([]userLoad, len(users))
	for i := range users {
		loads[i].row = UserUsage{ID: users[i].ID, Name: users[i].Name, Hosts: users[i].LiveHosts, LastActive: users[i].LastUpload, Daily: padDays(nil, from)}
	}
	err = forEach(idx2(len(users)), func(i int) error {
		u := users[i]
		var ids []string
		for id, label := range machines.Labels {
			if strings.HasPrefix(label, store.DeviceLabelPrefix(u.Email)) {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		slices.Sort(ids)
		l := &loads[i]
		us, err := src.Summary(ctx, from, to, url.Values{"machine": {strings.Join(ids, ",")}})
		if err != nil {
			return err
		}
		f := figures(us)
		l.row.Sessions, l.row.Tokens, l.row.Cost = f.Sessions, f.TokensIn+f.TokensOut, f.CostUSD
		l.row.Daily = padDays(f.Daily, from)
		if l.days, err = s.activeDays(ctx, src, from, to, strings.Join(ids, ",")); err != nil {
			return err
		}
		for _, id := range ids {
			d, err := s.activeDays(ctx, src, from, to, id)
			if err != nil {
				return err
			}
			l.hosts = append(l.hosts, d)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	org.Daily = padDays(org.Daily, from)
	data := &orgData{Org: org, Users: map[string]UserUsage{}}
	for _, l := range loads {
		if l.row.Sessions > 0 {
			org.ActiveUsers++
		}
		for _, h := range l.hosts {
			if len(h) > 0 {
				org.ActiveHosts++
			}
		}
		countActive(org.Daily, l.days, l.hosts)
		data.Users[l.row.ID] = l.row
	}
	return data, nil
}

func idx2(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func (s *Service) activeDays(ctx context.Context, src usage.Source, from, to, machine string) (map[string]bool, error) {
	byDay, err := src.SessionDays(ctx, from, to, machine)
	out := map[string]bool{}
	for d, n := range byDay {
		out[d] = n > 0
	}
	return out, err
}

// countActive adds one User (their active days) and each of their Hosts to the daily series.
func countActive(daily []Day, userDays map[string]bool, hostDays []map[string]bool) {
	for i := range daily {
		if userDays[daily[i].Date] {
			daily[i].ActiveUsers++
		}
		for _, h := range hostDays {
			if h[daily[i].Date] {
				daily[i].ActiveHosts++
			}
		}
	}
}

func dirSize(dir string) (n int64) {
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// Users are the Admin breakdown rows by User ID. A User not yet refreshed gets a zero row.
func (s *Service) Users(ctx context.Context) (map[string]UserUsage, error) {
	s.mu.Lock()
	d := s.orgCache
	s.mu.Unlock()
	users, err := s.st.StatsUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]UserUsage, len(users))
	from, _ := s.window()
	for _, u := range users {
		if row, ok := d.users()[u.ID]; ok {
			out[u.ID] = row
			continue
		}
		out[u.ID] = UserUsage{ID: u.ID, Name: u.Name, Hosts: u.LiveHosts, LastActive: u.LastUpload, Daily: padDays(nil, from)}
	}
	return out, nil
}

func (d *orgData) users() map[string]UserUsage {
	if d == nil {
		return nil
	}
	return d.Users
}

func (s *Service) zeroFigures() *Figures {
	from, _ := s.window()
	return &Figures{Daily: padDays(nil, from), Models: []Ranked{}, Agents: []Ranked{}}
}

func (s *Service) ownOrKick(userID string) *Figures {
	s.mu.Lock()
	f := s.own[userID]
	s.mu.Unlock()
	if f != nil {
		return f
	}
	s.kick(userID)
	return s.zeroFigures()
}

func (s *Service) kick(userID string) {
	s.mu.Lock()
	now := s.clk.Now()
	if s.inflight[userID] || now.Sub(s.lastKick[userID]) < kickEvery {
		s.mu.Unlock()
		return
	}
	s.inflight[userID], s.lastKick[userID] = true, now
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		f, err := s.loadOwn(ctx, userID)
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.inflight, userID)
		if err != nil {
			slog.Error("stats: user figures", "user", userID, "err", err)
			return
		}
		s.own[userID] = f
	}()
}

func (s *Service) loadOwn(ctx context.Context, userID string) (*Figures, error) {
	src, ok := s.source(userID)
	if !ok {
		return nil, errors.New("the Instance is not running")
	}
	from, to := s.window()
	sum, err := src.Summary(ctx, from, to, nil)
	if err != nil {
		return nil, err
	}
	f := figures(sum)
	byDay, err := src.SessionDays(ctx, from, to, "")
	if err != nil {
		return nil, err
	}
	for i := range f.Daily {
		f.Daily[i].Sessions = byDay[f.Daily[i].Date]
	}
	f.Daily = padDays(f.Daily, from)
	var machines struct {
		Labels map[string]string `json:"machine_labels"`
	}
	if err := src.Get(ctx, "machines", url.Values{}, &machines); err != nil {
		return nil, err
	}
	var hostDays []map[string]bool
	for id := range machines.Labels {
		d, err := s.activeDays(ctx, src, from, to, id)
		if err != nil {
			return nil, err
		}
		hostDays = append(hostDays, d)
	}
	userDays := map[string]bool{}
	for d, n := range byDay {
		userDays[d] = n > 0
	}
	countActive(f.Daily, userDays, hostDays)
	users, err := s.st.StatsUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.ID == userID {
			f.ActiveHosts = u.LiveHosts
		}
	}
	if f.Sessions > 0 {
		f.ActiveUsers = 1
	}
	return f, nil
}

func (s *Service) team(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authn.Principal(r)
	if !ok {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	u, err := s.st.UserByID(r.Context(), p.Effective())
	if err != nil || u.Disabled {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	d := s.orgCache
	s.mu.Unlock()
	org := s.zeroFigures()
	if d != nil {
		org = d.Org
	}
	from, to := s.window()
	httpx.JSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "org": org, "you": s.ownOrKick(u.ID)})
}
