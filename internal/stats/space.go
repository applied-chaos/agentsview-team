package stats

import (
	"context"
	"path/filepath"
)

// Space is space used in bytes: for each User their Instance database plus data directory.
// The Admin Instance and the replay spool are separate lines; Total adds everything.
type Space struct {
	Users         map[string]int64 `json:"-"`
	AdminInstance int64            `json:"adminInstance"`
	Spool         int64            `json:"spool"`
	Total         int64            `json:"total"`
}

// Space returns the cached snapshot, empty until the first refresh has computed one.
func (s *Service) Space(context.Context) (*Space, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.space == nil {
		return &Space{Users: map[string]int64{}}, nil
	}
	return s.space, nil
}

// UserSpace is one User's space used.
func (s *Service) UserSpace(ctx context.Context, userID string) (int64, error) {
	sp, err := s.Space(ctx)
	if err != nil {
		return 0, err
	}
	return sp.Users[userID], nil
}

func (s *Service) computeSpace(ctx context.Context) (*Space, error) {
	users, err := s.st.StatsUsers(ctx)
	if err != nil {
		return nil, err
	}
	dbs, err := s.st.InstanceDBSizes(ctx)
	if err != nil {
		return nil, err
	}
	of := func(id string) int64 { return dbs[id] + dirSize(filepath.Join(s.dataDir, "instances", id)) }
	sp := &Space{Users: map[string]int64{}, AdminInstance: of(s.inst.AdminID()), Spool: dirSize(filepath.Join(s.dataDir, "spool"))}
	sp.Total = sp.AdminInstance + sp.Spool
	for _, u := range users {
		sp.Users[u.ID] = of(u.ID)
		sp.Total += sp.Users[u.ID]
	}
	return sp, nil
}
