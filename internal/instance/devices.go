package instance

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/applied-chaos/agentsview-team/internal/tokens"
)

// ownerTx runs fn in a transaction on the User's Instance database as its owner, scoped to
// the Instance's tenant (upstream forces row-level security, even for the owner).
func (m *Manager) ownerTx(ctx context.Context, userID string, fn func(pgx.Tx) error) error {
	sec, err := m.loadSecrets(ctx, userID)
	if err != nil {
		return err
	}
	n := namesFor(userID)
	conn, err := pgx.Connect(ctx, m.dbURL(n.owner, sec.ownerPW, n.db))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `select set_config('agentsview.tenant_id', $1, true)`, userID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EnrolDevice registers a raw-sync device in the User's Instance. Upstream has no enrolment
// endpoint, so this writes its raw_devices row (spike/enrol.sh); only a hash of the
// Credential is stored.
func (m *Manager) EnrolDevice(ctx context.Context, userID, displayName string) (deviceID, credential string, err error) {
	deviceID, credential, err = tokens.NewDevice()
	if err != nil {
		return "", "", err
	}
	err = m.ownerTx(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into agentsview.raw_devices (device_id, tenant_id, display_name, credential_sha256, created_at)
			values ($1, $2, $3, sha256(convert_to($4, 'UTF8')), now())`, deviceID, userID, displayName, credential)
		return err
	})
	return deviceID, credential, err
}

// DeviceForToken resolves a device access token (avdt_...) issued by the User's Instance.
func (m *Manager) DeviceForToken(ctx context.Context, userID, token string) (string, error) {
	var device string
	err := m.ownerTx(ctx, userID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select device_id from agentsview.raw_device_tokens where token_sha256 = sha256(convert_to($1, 'UTF8'))`,
			token).Scan(&device)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return device, err
}

func (m *Manager) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	return m.ownerTx(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update agentsview.raw_devices set revoked_at = coalesce(revoked_at, now()) where device_id = $1`, deviceID)
		return err
	})
}

// UpsertDevice creates a device with a known Credential, or renames it when it exists, so a
// retry after a crash converges. Replay uses it for the Admin Instance's mirrored devices.
func (m *Manager) UpsertDevice(ctx context.Context, tenantID, deviceID, credential, displayName string) error {
	return m.ownerTx(ctx, tenantID, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `update agentsview.raw_devices set display_name = $3 where tenant_id = $1 and device_id = $2`,
			tenantID, deviceID, displayName)
		if err != nil || ct.RowsAffected() > 0 {
			return err
		}
		_, err = tx.Exec(ctx, `
			insert into agentsview.raw_devices (device_id, tenant_id, display_name, credential_sha256, created_at)
			values ($1, $2, $3, sha256(convert_to($4, 'UTF8')), now())`, deviceID, tenantID, displayName, credential)
		return err
	})
}
