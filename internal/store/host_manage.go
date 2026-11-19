package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrHostNotFound is an unknown Host or one owned by someone else.
var ErrHostNotFound = errors.New("host not found")

// ErrHostName is a name already used by another live Host of the User, or a revoked Host.
var ErrHostName = errors.New("host name unavailable")

func lockHost(ctx context.Context, tx pgx.Tx, userID, hostID string) (Host, error) {
	h, err := scanHost(tx.QueryRow(ctx, `select `+hostCols+` from hosts where id::text = $2 and user_id = $1::uuid for update`, userID, hostID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Host{}, ErrHostNotFound
	}
	return h, err
}

func (s *Store) HostOf(ctx context.Context, userID, hostID string) (Host, error) {
	h, err := scanHost(s.pool.QueryRow(ctx, `select `+hostCols+` from hosts where id::text = $2 and user_id = $1::uuid`, userID, hostID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Host{}, ErrHostNotFound
	}
	return h, err
}

func (s *Store) HostByDevice(ctx context.Context, deviceID string) (Host, error) {
	h, err := scanHost(s.pool.QueryRow(ctx, `select `+hostCols+` from hosts where device_id = $1`, deviceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Host{}, ErrHostNotFound
	}
	return h, err
}

// RenameHost renames a live Host; apply renames its device in the Instance inside the same
// transaction, so a failure there leaves the name unchanged.
func (s *Store) RenameHost(ctx context.Context, userID, hostID, name string, apply func(deviceID string) error) (before, after Host, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Host{}, Host{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	before, err = lockHost(ctx, tx, userID, hostID)
	if err != nil {
		return Host{}, Host{}, err
	}
	if before.RevokedAt != nil {
		return Host{}, Host{}, ErrHostName
	}
	if before.Name == name {
		return before, before, nil
	}
	after, err = scanHost(tx.QueryRow(ctx, `update hosts set name = $2 where id = $1::uuid returning `+hostCols, before.ID, name))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Host{}, Host{}, ErrHostName
	}
	if err != nil {
		return Host{}, Host{}, err
	}
	if err := apply(after.DeviceID); err != nil {
		return Host{}, Host{}, err
	}
	return before, after, tx.Commit(ctx)
}

// RevokeUserHost marks the Host revoked, calling apply to revoke its device first. Safe to
// repeat: apply is idempotent, so a crash between the two steps is repaired by the next call.
func (s *Store) RevokeUserHost(ctx context.Context, userID, hostID string, now time.Time, apply func(deviceID string) error) (Host, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Host{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	h, err := lockHost(ctx, tx, userID, hostID)
	if err != nil {
		return Host{}, err
	}
	if err := apply(h.DeviceID); err != nil {
		return Host{}, err
	}
	if h.RevokedAt != nil {
		return h, nil
	}
	h, err = scanHost(tx.QueryRow(ctx, `update hosts set revoked_at = $2 where id = $1::uuid returning `+hostCols, h.ID, now))
	if err != nil {
		return Host{}, err
	}
	return h, tx.Commit(ctx)
}

// DeleteUserHost removes the Host, calling apply to revoke its device first; uploaded sessions
// stay in the Instance. A crash between the two steps is repaired by the next call.
func (s *Store) DeleteUserHost(ctx context.Context, userID, hostID string, apply func(deviceID string) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	h, err := lockHost(ctx, tx, userID, hostID)
	if err != nil {
		return err
	}
	if err := apply(h.DeviceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from hosts where id = $1::uuid`, h.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReportHostVersion(ctx context.Context, deviceID, version string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		update hosts set client_version = $2, last_seen_at = greatest(coalesce(last_seen_at, $3), $3)
		where device_id = $1 and revoked_at is null`, deviceID, version, now)
	return err
}
