package instance

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (m *Manager) RenameDevice(ctx context.Context, userID, deviceID, displayName string) error {
	return m.ownerTx(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update agentsview.raw_devices set display_name = $3 where tenant_id = $1 and device_id = $2`,
			userID, deviceID, displayName)
		return err
	})
}

func (m *Manager) CredentialValid(ctx context.Context, userID, deviceID, credential string) (bool, error) {
	var ok bool
	err := m.ownerTx(ctx, userID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select exists (select 1 from agentsview.raw_devices where tenant_id = $1 and device_id = $2
			and revoked_at is null and credential_sha256 = sha256(convert_to($3, 'UTF8')))`, userID, deviceID, credential).Scan(&ok)
	})
	return ok, err
}
