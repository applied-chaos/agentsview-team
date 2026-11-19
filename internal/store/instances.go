package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// InstanceRow holds an Instance's sealed secrets.
type InstanceRow struct {
	UserID          string
	DBName          string
	OwnerPassword   []byte
	RuntimePassword []byte
	BearerToken     []byte
	CursorSecret    []byte
}

// CreateInstance stores a new row unless the User already has one, so a retry
// after a crash keeps the secrets the first run chose.
func (s *Store) CreateInstance(ctx context.Context, r InstanceRow) error {
	_, err := s.pool.Exec(ctx, `
		insert into instances (user_id, db_name, owner_password, runtime_password, bearer_token, cursor_secret)
		values ($1::uuid, $2, $3, $4, $5, $6) on conflict (user_id) do nothing`,
		r.UserID, r.DBName, r.OwnerPassword, r.RuntimePassword, r.BearerToken, r.CursorSecret)
	return err
}

func (s *Store) Instance(ctx context.Context, userID string) (InstanceRow, bool, error) {
	var r InstanceRow
	err := s.pool.QueryRow(ctx, `
		select user_id::text, db_name, owner_password, runtime_password, bearer_token, cursor_secret
		from instances where user_id = $1::uuid
		union all
		select id::text, db_name, owner_password, runtime_password, bearer_token, cursor_secret
		from admin_instance where id = $1::uuid`, userID).
		Scan(&r.UserID, &r.DBName, &r.OwnerPassword, &r.RuntimePassword, &r.BearerToken, &r.CursorSecret)
	if errors.Is(err, pgx.ErrNoRows) {
		return InstanceRow{}, false, nil
	}
	return r, err == nil, err
}

// EnabledInstanceUserIDs lists Users whose Instance should run at start-up.
func (s *Store) EnabledInstanceUserIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		select i.user_id::text from instances i join users u on u.id = i.user_id where not u.disabled`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
