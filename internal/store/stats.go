package store

import (
	"context"
	"time"
)

type StatsUser struct {
	ID, Email, Name string
	LiveHosts       int
	LastUpload      *time.Time
}

func (s *Store) StatsUsers(ctx context.Context) ([]StatsUser, error) {
	rows, err := s.pool.Query(ctx, `
		select u.id::text, u.email, u.name, count(h.id) filter (where h.revoked_at is null), max(h.last_upload_at)
		from users u left join hosts h on h.user_id = u.id group by u.id order by u.email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatsUser
	for rows.Next() {
		var u StatsUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.LiveHosts, &u.LastUpload); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// InstanceDBSizes is pg_database_size of each Instance database by owner ID, the Admin
// Instance included. An Instance whose database is not created yet is absent.
func (s *Store) InstanceDBSizes(ctx context.Context) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx, `
		select i.id, pg_database_size(d.datname) from (
			select user_id::text id, db_name from instances union all select id::text, db_name from admin_instance
		) i join pg_database d on d.datname = i.db_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
