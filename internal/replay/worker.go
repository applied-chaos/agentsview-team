package replay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/secrets"
	"github.com/applied-chaos/agentsview-team/internal/store"
	"github.com/applied-chaos/agentsview-team/internal/tokens"
)

const (
	batch    = 200
	idleTick = 2 * time.Second
)

type Admin interface {
	AdminID() string
	Target(id string) (base *url.URL, token string, ok bool)
	UpsertDevice(ctx context.Context, tenantID, deviceID, credential, displayName string) error
	RevokeDevice(ctx context.Context, tenantID, deviceID string) error
}

type Service struct {
	st    *store.Store
	box   *secrets.Box
	admin Admin
	clk   *clock.Clock
	spool *Spool
	tee   *Tee

	nudge chan struct{}

	// replayMu serialises the worker and Drain so one manifest is never replayed twice at once.
	replayMu sync.Mutex
	mu       sync.Mutex
	clients  map[string]*client
	hooks    testHooks
	root     context.Context
	stop     context.CancelFunc
	done     chan struct{}
}

func New(st *store.Store, box *secrets.Box, admin Admin, dev Devices, clk *clock.Clock, spoolDir string) (*Service, error) {
	sp, err := NewSpool(spoolDir)
	if err != nil {
		return nil, err
	}
	s := &Service{st: st, box: box, admin: admin, clk: clk, spool: sp, nudge: make(chan struct{}, 1), clients: map[string]*client{}}
	s.tee = &Tee{spool: sp, st: st, dev: dev, clk: clk, nudge: s.wake}
	return s, nil
}

func (s *Service) Tee() *Tee { return s.tee }

func (s *Service) wake() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}

// Start runs the worker until ctx ends or Stop is called; state is all in Postgres and the spool,
// so a fresh worker resumes exactly where the last one stopped.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		return
	}
	s.root = ctx
	ctx, s.stop = context.WithCancel(ctx)
	s.done = make(chan struct{})
	go func(done chan struct{}) {
		defer close(done)
		s.run(ctx)
	}(s.done)
}

func (s *Service) Stop() {
	s.mu.Lock()
	stop, done := s.stop, s.done
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := s.pass(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("replay pass", "err", err)
		}
		select {
		case <-ctx.Done():
		case <-s.nudge:
		case <-time.After(idleTick):
		}
	}
}

// pass renames drifted devices, then replays pending manifests in commit order. A manifest that
// fails holds back later manifests of the same source only.
func (s *Service) pass(ctx context.Context) error {
	if _, _, ok := s.admin.Target(s.admin.AdminID()); !ok {
		return nil
	}
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	if err := s.rename(ctx); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	pending, err := s.st.PendingManifests(ctx, batch)
	if err != nil {
		return err
	}
	blocked := map[string]bool{}
	for _, p := range pending {
		key, err := s.replayOne(ctx, p, blocked)
		if s.hooks.isCrash(err) {
			return err
		}
		if err != nil {
			slog.Warn("replay manifest", "id", p.ID, "err", err)
			blocked[key] = true
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	if len(pending) == batch {
		s.wake()
	}
	return nil
}

// Drain replays every pending manifest of one User, so a Delete loses none of their data.
func (s *Service) Drain(ctx context.Context, userID string) error {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	for {
		pending, err := s.st.PendingManifestsFor(ctx, userID, batch)
		if err != nil || len(pending) == 0 {
			return err
		}
		if _, _, ok := s.admin.Target(s.admin.AdminID()); !ok {
			return errors.New("admin instance not running")
		}
		blocked := map[string]bool{}
		for _, p := range pending {
			if _, err := s.replayOne(ctx, p, blocked); err != nil {
				return fmt.Errorf("replay manifest %d: %w", p.ID, err)
			}
		}
	}
}

func (s *Service) rename(ctx context.Context) error {
	devs, want, err := s.st.ReplayDrift(ctx)
	if err != nil {
		return err
	}
	for i, d := range devs {
		cred, err := s.credential(d)
		if err != nil {
			return err
		}
		if err := s.admin.UpsertDevice(ctx, s.admin.AdminID(), d.DeviceID, cred, want[i]); err != nil {
			return err
		}
		if err := s.st.SetReplayDeviceName(ctx, d.HostID, want[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) credential(d store.ReplayDevice) (string, error) {
	plain, err := s.box.Open("replay/"+d.HostID+"/credential", d.Credential)
	return string(plain), err
}

// mirror returns the Host's mirrored device, creating it on first use. The row is stored before
// the device exists in the Admin Instance and the upsert is idempotent, so a crash between the two converges.
func (s *Service) mirror(ctx context.Context, h store.ReplayHost) (store.ReplayDevice, string, error) {
	deviceID, credential, err := tokens.NewDevice()
	if err != nil {
		return store.ReplayDevice{}, "", err
	}
	sealed, err := s.box.Seal("replay/"+h.ID+"/credential", []byte(credential))
	if err != nil {
		return store.ReplayDevice{}, "", err
	}
	d, err := s.st.EnsureReplayDevice(ctx, store.ReplayDevice{HostID: h.ID, DeviceID: deviceID, Credential: sealed})
	if err != nil {
		return d, "", err
	}
	cred, err := s.credential(d)
	if err != nil {
		return d, "", err
	}
	if d.Name != h.Label {
		if err := s.admin.UpsertDevice(ctx, s.admin.AdminID(), d.DeviceID, cred, h.Label); err != nil {
			return d, "", err
		}
		if err := s.st.SetReplayDeviceName(ctx, h.ID, h.Label); err != nil {
			return d, "", err
		}
	}
	return d, cred, nil
}

func (s *Service) client(d store.ReplayDevice, cred string) (*client, error) {
	target, _, ok := s.admin.Target(s.admin.AdminID())
	if !ok {
		return nil, errors.New("admin instance not running")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.clients[d.DeviceID]
	if c == nil || c.target != target.String() {
		c = newClient(target, s.admin.AdminID(), d.DeviceID, cred)
		s.clients[d.DeviceID] = c
	}
	return c, nil
}

type manifestView struct {
	Provider string `json:"provider"`
	Root     string `json:"configured_root_id"`
	Source   string `json:"source_key"`
	Entries  []struct {
		Objects []ObjectRef `json:"objects"`
	} `json:"entries"`
}

// replayOne copies a manifest's objects to the Admin Instance and commits the manifest on the
// Admin Instance's own receipt chain. The parent receipt comes from our record taken before the
// commit, so a retry after a crash sends the same bytes and upstream answers idempotently.
func (s *Service) replayOne(ctx context.Context, p store.PendingManifest, blocked map[string]bool) (string, error) {
	var mv manifestView
	var raw map[string]json.RawMessage
	if json.Unmarshal(p.Body, &mv) != nil || json.Unmarshal(p.Body, &raw) != nil {
		slog.Error("replay: unreadable manifest, skipping", "id", p.ID)
		return "", s.st.MarkReplayed(ctx, p.ID, s.clk.Now())
	}
	src := sha256.Sum256([]byte(mv.Source))
	key := p.DeviceID + "\x00" + mv.Provider + "\x00" + mv.Root + "\x00" + string(src[:])
	if blocked[key] {
		return key, errors.New("an earlier manifest of this source is waiting")
	}
	host, ok, err := s.st.ReplayHostByDevice(ctx, p.DeviceID)
	if err != nil {
		return key, err
	}
	if !ok {
		slog.Warn("replay: manifest from a device with no Host, skipping", "id", p.ID, "device", p.DeviceID)
		return key, s.st.MarkReplayed(ctx, p.ID, s.clk.Now())
	}
	dev, cred, err := s.mirror(ctx, host)
	if err != nil {
		return key, err
	}
	c, err := s.client(dev, cred)
	if err != nil {
		return key, err
	}

	seen := map[ObjectRef]bool{}
	var refs []ObjectRef
	for _, e := range mv.Entries {
		for _, o := range e.Objects {
			if !seen[o] {
				seen[o] = true
				refs = append(refs, o)
			}
		}
	}
	missing, err := c.Missing(ctx, mv.Provider, refs)
	if err != nil {
		return key, err
	}
	for _, o := range missing {
		f, err := s.spool.Open(o)
		if err != nil {
			return key, fmt.Errorf("object %s not in the spool: %w", o.file(), err)
		}
		err = c.Upload(ctx, mv.Provider, o, f)
		_ = f.Close()
		if err != nil {
			return key, err
		}
	}

	var receipt string
	for range 3 {
		parent, err := s.st.ReplayHead(ctx, dev.DeviceID, mv.Provider, mv.Root, src[:])
		if err != nil {
			return key, err
		}
		if parent == "" {
			delete(raw, "expected_parent_receipt")
		} else {
			raw["expected_parent_receipt"], _ = json.Marshal(parent)
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return key, err
		}
		receipt, err = c.Commit(ctx, body)
		var cf *conflict
		if errors.As(err, &cf) && cf.Code == "head_conflict" && cf.CurrentReceipt != "" {
			// Our chain record is behind (lost on a crash window): adopt the server's head.
			if err := s.st.SetReplayHead(ctx, dev.DeviceID, mv.Provider, mv.Root, src[:], cf.CurrentReceipt); err != nil {
				return key, err
			}
			continue
		}
		if err != nil {
			return key, err
		}
		break
	}
	if receipt == "" {
		return key, errors.New("could not settle the Admin Instance's receipt chain")
	}
	if err := s.hooks.crashAfterCommit(); err != nil {
		return key, err
	}
	if err := s.st.CompleteReplay(ctx, p.ID, dev.DeviceID, mv.Provider, mv.Root, src[:], receipt, s.clk.Now()); err != nil {
		return key, err
	}
	s.prune(ctx, refs)
	return key, nil
}

// prune drops spooled objects no unreplayed manifest still needs (ADR 0003). A failure only
// leaves a file behind; a crash before this runs strands that manifest's objects.
func (s *Service) prune(ctx context.Context, refs []ObjectRef) {
	for _, o := range refs {
		wanted, err := s.st.ObjectStillWanted(ctx, o.SHA256)
		if err != nil || wanted {
			continue
		}
		if err := s.spool.Remove(o); err != nil {
			slog.Warn("replay: prune spool object", "object", o.file(), "err", err)
		}
	}
}
