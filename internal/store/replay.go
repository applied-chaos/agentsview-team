package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) AdminInstance(ctx context.Context) (id string, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `select id::text from admin_instance`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (s *Store) CreateAdminInstance(ctx context.Context, r InstanceRow) error {
	_, err := s.pool.Exec(ctx, `
		insert into admin_instance (id, db_name, owner_password, runtime_password, bearer_token, cursor_secret)
		values ($1::uuid, $2, $3, $4, $5, $6) on conflict (singleton) do nothing`,
		r.UserID, r.DBName, r.OwnerPassword, r.RuntimePassword, r.BearerToken, r.CursorSecret)
	return err
}

type PendingManifest struct {
	ID       int64
	UserID   string
	DeviceID string
	Body     []byte
}

func (s *Store) SpoolManifest(ctx context.Context, userID, deviceID, manifestID string, body []byte, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into replay_manifests (user_id, device_id, manifest_id, body, received_at)
		values ($1::uuid, $2, $3, $4, $5) on conflict (user_id, manifest_id) do nothing`,
		userID, deviceID, manifestID, body, now)
	return err
}

func (s *Store) PendingManifests(ctx context.Context, limit int) ([]PendingManifest, error) {
	rows, err := s.pool.Query(ctx, `
		select id, user_id::text, device_id, body from replay_manifests
		where replayed_at is null order by id limit $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (p PendingManifest, err error) {
		err = r.Scan(&p.ID, &p.UserID, &p.DeviceID, &p.Body)
		return
	})
}

// ReplayHost is a Host as replay needs it: its display name in the Admin Instance.
type ReplayHost struct {
	ID    string
	Label string // DeviceLabel(user email, host name)
}

func (s *Store) ReplayHostByDevice(ctx context.Context, deviceID string) (ReplayHost, bool, error) {
	var h ReplayHost
	var email, name string
	err := s.pool.QueryRow(ctx, `
		select h.id::text, u.email, h.name from hosts h join users u on u.id = h.user_id
		where h.device_id = $1`, deviceID).Scan(&h.ID, &email, &name)
	h.Label = DeviceLabel(email, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplayHost{}, false, nil
	}
	return h, err == nil, err
}

// DeviceLabel is a Host's display name in the Admin Instance; usage is grouped back to Users by
// DeviceLabelPrefix, and replay renames devices to match.
func DeviceLabel(email, host string) string { return DeviceLabelPrefix(email) + host }

func DeviceLabelPrefix(email string) string { return email + "/" }

type ReplayDevice struct {
	HostID, DeviceID, Name string
	Credential             []byte
}

func (s *Store) EnsureReplayDevice(ctx context.Context, d ReplayDevice) (ReplayDevice, error) {
	_, err := s.pool.Exec(ctx, `
		insert into replay_devices (host_id, device_id, credential) values ($1::uuid, $2, $3)
		on conflict (host_id) do nothing`, d.HostID, d.DeviceID, d.Credential)
	if err != nil {
		return ReplayDevice{}, err
	}
	err = s.pool.QueryRow(ctx, `select host_id::text, device_id, credential, name from replay_devices where host_id = $1::uuid`, d.HostID).
		Scan(&d.HostID, &d.DeviceID, &d.Credential, &d.Name)
	return d, err
}

func (s *Store) SetReplayDeviceName(ctx context.Context, hostID, name string) error {
	_, err := s.pool.Exec(ctx, `update replay_devices set name = $2 where host_id = $1::uuid`, hostID, name)
	return err
}

func (s *Store) ReplayDrift(ctx context.Context) ([]ReplayDevice, []string, error) {
	rows, err := s.pool.Query(ctx, `
		select d.host_id::text, d.device_id, d.credential, d.name, u.email || '/' || h.name
		from replay_devices d join hosts h on h.id = d.host_id join users u on u.id = h.user_id
		where d.name <> u.email || '/' || h.name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var devs []ReplayDevice
	var want []string
	for rows.Next() {
		var d ReplayDevice
		var w string
		if err := rows.Scan(&d.HostID, &d.DeviceID, &d.Credential, &d.Name, &w); err != nil {
			return nil, nil, err
		}
		devs, want = append(devs, d), append(want, w)
	}
	return devs, want, rows.Err()
}

func (s *Store) ReplayHead(ctx context.Context, deviceID, provider, root string, sourceHash []byte) (string, error) {
	var receipt string
	err := s.pool.QueryRow(ctx, `
		select receipt from replay_heads where device_id = $1 and provider = $2 and root_id = $3 and source_hash = $4`,
		deviceID, provider, root, sourceHash).Scan(&receipt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return receipt, err
}

func (s *Store) CompleteReplay(ctx context.Context, id int64, deviceID, provider, root string, sourceHash []byte, receipt string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `
		insert into replay_heads (device_id, provider, root_id, source_hash, receipt) values ($1, $2, $3, $4, $5)
		on conflict (device_id, provider, root_id, source_hash) do update set receipt = excluded.receipt`,
		deviceID, provider, root, sourceHash, receipt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update replay_manifests set replayed_at = $2 where id = $1`, id, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkReplayed(ctx context.Context, id int64, now time.Time) error {
	_, err := s.pool.Exec(ctx, `update replay_manifests set replayed_at = $2 where id = $1`, id, now)
	return err
}

func (s *Store) SetReplayHead(ctx context.Context, deviceID, provider, root string, sourceHash []byte, receipt string) error {
	_, err := s.pool.Exec(ctx, `
		insert into replay_heads (device_id, provider, root_id, source_hash, receipt) values ($1, $2, $3, $4, $5)
		on conflict (device_id, provider, root_id, source_hash) do update set receipt = excluded.receipt`,
		deviceID, provider, root, sourceHash, receipt)
	return err
}

// ObjectStillWanted reports whether an unreplayed manifest references the object (by its SHA-256).
func (s *Store) ObjectStillWanted(ctx context.Context, sha string) (bool, error) {
	var wanted bool
	err := s.pool.QueryRow(ctx, `
		select exists (select 1 from replay_manifests where replayed_at is null and position(convert_to($1, 'UTF8') in body) > 0)`, sha).Scan(&wanted)
	return wanted, err
}

func (s *Store) PendingManifestsFor(ctx context.Context, userID string, limit int) ([]PendingManifest, error) {
	rows, err := s.pool.Query(ctx, `
		select id, user_id::text, device_id, body from replay_manifests
		where replayed_at is null and user_id = $1::uuid order by id limit $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (p PendingManifest, err error) {
		err = r.Scan(&p.ID, &p.UserID, &p.DeviceID, &p.Body)
		return
	})
}
