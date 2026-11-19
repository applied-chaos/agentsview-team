package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// InstallMode is how a Host runs the agent; the hosts table constrains it to these.
type InstallMode string

const (
	ModeSystemd InstallMode = "systemd"
	ModeProfile InstallMode = "profile"
	ModeLaunchd InstallMode = "launchd"
	ModeTask    InstallMode = "task"
)

// ErrInstallToken covers unknown, used, expired and Disabled-User tokens.
var ErrInstallToken = errors.New("invalid install token")

type Host struct {
	ID            string
	UserID        string
	Name          string
	DeviceID      string
	OS            string
	InstallMode   InstallMode
	ClientVersion string
	CreatedAt     time.Time
	LastSeenAt    *time.Time
	LastUploadAt  *time.Time
	RevokedAt     *time.Time
	Ephemeral     bool
}

const hostCols = `id::text, user_id::text, name, device_id, os, install_mode, client_version,
	created_at, last_seen_at, last_upload_at, revoked_at, ephemeral`

func scanHost(row pgx.Row) (Host, error) {
	var h Host
	err := row.Scan(&h.ID, &h.UserID, &h.Name, &h.DeviceID, &h.OS, &h.InstallMode, &h.ClientVersion,
		&h.CreatedAt, &h.LastSeenAt, &h.LastUploadAt, &h.RevokedAt, &h.Ephemeral)
	return h, err
}

type TokenOpts struct {
	Description         string
	Reusable, Ephemeral bool
}

func (s *Store) CreateInstallToken(ctx context.Context, userID string, hash []byte, now time.Time, expires *time.Time, o TokenOpts) (id string, err error) {
	err = s.pool.QueryRow(ctx, `
		insert into install_tokens (user_id, token_hash, expires_at, created_at, description, reusable, ephemeral)
		values ($1::uuid, $2, $3, $4, $5, $6, $7) returning id::text`,
		userID, hash, expires, now, o.Description, o.Reusable, o.Ephemeral).Scan(&id)
	return id, err
}

// ClaimInstallToken checks a token for an enabled User and spends it unless it is reusable.
func (s *Store) ClaimInstallToken(ctx context.Context, hash []byte, now time.Time) (tokenID, userID string, err error) {
	err = s.pool.QueryRow(ctx, `
		update install_tokens t set used_at = case when t.reusable then t.used_at else $2 end
		from users u
		where t.token_hash = $1 and (t.reusable or t.used_at is null) and t.revoked_at is null and (t.expires_at is null or t.expires_at > $2)
			and u.id = t.user_id and not u.disabled
		returning t.id::text, t.user_id::text`, hash, now).Scan(&tokenID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrInstallToken
	}
	return tokenID, userID, err
}

// InstallToken is a listed token; the secret is never recoverable.
type InstallToken struct {
	ID, Description     string
	Reusable, Ephemeral bool
	CreatedAt           time.Time
	ExpiresAt           *time.Time // nil: never
	LastUsedAt          *time.Time
	HostsCreated        int
}

// InstallTokensOf lists the User's tokens that can still enrol a Host, newest first.
func (s *Store) InstallTokensOf(ctx context.Context, userID string, now time.Time) ([]InstallToken, error) {
	rows, err := s.pool.Query(ctx, `
		select t.id::text, t.description, t.reusable, t.ephemeral, t.created_at, t.expires_at, t.last_used_at,
			(select count(*) from hosts h where h.install_token_id = t.id)
		from install_tokens t
		where t.user_id = $1::uuid and t.revoked_at is null and (t.expires_at is null or t.expires_at > $2) and (t.reusable or t.used_at is null)
		order by t.created_at desc`, userID, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (t InstallToken, err error) {
		err = r.Scan(&t.ID, &t.Description, &t.Reusable, &t.Ephemeral, &t.CreatedAt, &t.ExpiresAt, &t.LastUsedAt, &t.HostsCreated)
		return t, err
	})
}

// RevokeInstallToken stops a token enrolling more Hosts; existing Hosts are untouched.
// It is idempotent and ErrInstallToken for another User's token.
func (s *Store) RevokeInstallToken(ctx context.Context, userID, tokenID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		update install_tokens set revoked_at = coalesce(revoked_at, $3) where id = $1::uuid and user_id = $2::uuid`,
		tokenID, userID, now)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrInstallToken
	}
	return err
}

// ReleaseInstallToken un-spends a token whose enrolment failed, so the command can be retried.
func (s *Store) ReleaseInstallToken(ctx context.Context, tokenID string) error {
	_, err := s.pool.Exec(ctx, `update install_tokens set used_at = null where id = $1::uuid and host_id is null`, tokenID)
	return err
}

// NewHost is what the script reports at enrolment.
type NewHost struct {
	Name, OS, ClientVersion string
	InstallMode             InstallMode
}

// CreateHost allocates a name unique among the User's live Hosts (suffix -2, -3 on a clash),
// calls enrol with it to create the device in the User's Instance, then stores the Host and
// links the token. The per-User lock keeps concurrent enrolments from choosing the same name.
func (s *Store) CreateHost(ctx context.Context, userID, tokenID string, n NewHost, now time.Time,
	enrol func(name string) (deviceID string, err error)) (Host, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Host{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 727003))`, userID); err != nil {
		return Host{}, err
	}
	name := n.Name
	for i := 2; ; i++ {
		var taken bool
		if err := tx.QueryRow(ctx, `select exists (select 1 from hosts where user_id = $1::uuid and lower(name) = lower($2) and revoked_at is null)`,
			userID, name).Scan(&taken); err != nil {
			return Host{}, err
		}
		if !taken {
			break
		}
		name = fmt.Sprintf("%s-%d", n.Name, i)
	}
	deviceID, err := enrol(name)
	if err != nil {
		return Host{}, err
	}
	h, err := scanHost(tx.QueryRow(ctx, `
		insert into hosts (user_id, name, device_id, os, install_mode, client_version, created_at, last_seen_at, install_token_id, ephemeral)
		values ($1::uuid, $2, $3, $4, $5, $6, $7, $7, $8::uuid, (select ephemeral from install_tokens where id = $8::uuid))
		returning `+hostCols,
		userID, name, deviceID, n.OS, n.InstallMode, n.ClientVersion, now, tokenID))
	if err != nil {
		return Host{}, err
	}
	if _, err := tx.Exec(ctx, `
		update install_tokens set last_used_at = $3, host_id = case when reusable then null else $2::uuid end
		where id = $1::uuid`, tokenID, h.ID, now); err != nil {
		return Host{}, err
	}
	return h, tx.Commit(ctx)
}

// HostsOf lists the User's Hosts, newest first.
func (s *Store) HostsOf(ctx context.Context, userID string) ([]Host, error) {
	rows, err := s.pool.Query(ctx, `select `+hostCols+` from hosts where user_id = $1::uuid order by created_at desc`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Host, error) { return scanHost(r) })
}

// TokenHost reports the state of one of the User's tokens: the linked Host once redeemed.
func (s *Store) TokenHost(ctx context.Context, userID, tokenID string, now time.Time) (state string, h *Host, err error) {
	var expires *time.Time
	var hostID *string
	var revoked bool
	err = s.pool.QueryRow(ctx, `select expires_at, host_id::text, revoked_at is not null from install_tokens where id = $1::uuid and user_id = $2::uuid`,
		tokenID, userID).Scan(&expires, &hostID, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrInstallToken
	}
	if err != nil {
		return "", nil, err
	}
	if hostID != nil {
		host, err := scanHost(s.pool.QueryRow(ctx, `select `+hostCols+` from hosts where id = $1::uuid`, *hostID))
		return "connected", &host, err
	}
	if revoked {
		return "revoked", nil, nil
	}
	if expires != nil && !expires.After(now) {
		return "expired", nil, nil
	}
	return "waiting", nil, nil
}

// TouchHostSeen records contact from a device; unknown devices are ignored.
func (s *Store) TouchHostSeen(ctx context.Context, deviceID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `update hosts set last_seen_at = greatest(coalesce(last_seen_at, $2), $2) where device_id = $1`, deviceID, now)
	return err
}

// TouchHostUpload records an accepted upload (which is also contact).
func (s *Store) TouchHostUpload(ctx context.Context, deviceID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		update hosts set last_upload_at = $2, last_seen_at = greatest(coalesce(last_seen_at, $2), $2) where device_id = $1`, deviceID, now)
	return err
}

func (s *Store) StaleEphemeral(ctx context.Context, cutoff time.Time) ([]Host, error) {
	rows, err := s.pool.Query(ctx, `select `+hostCols+` from hosts
		where ephemeral and revoked_at is null and coalesce(last_seen_at, created_at) < $1`, cutoff)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Host, error) { return scanHost(r) })
}

// RevokeHost marks a Host revoked; a second call keeps the first time.
func (s *Store) RevokeHost(ctx context.Context, hostID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `update hosts set revoked_at = coalesce(revoked_at, $2) where id = $1::uuid`, hostID, now)
	return err
}
