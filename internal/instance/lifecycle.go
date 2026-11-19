package instance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Disable stops the User's Instance and revokes its devices; it is safe to repeat, so a Disable
// that crashed half way is finished by the next one.
func (m *Manager) Disable(ctx context.Context, userID string) error {
	m.Stop(userID)
	return m.RevokeDevices(ctx, userID)
}

// RevokeDevices revokes every raw-sync device in the User's Instance, so Host Credentials stop
// working even if the Instance is reached some other way. Nothing to do before first provisioning.
func (m *Manager) RevokeDevices(ctx context.Context, userID string) error {
	_, ok, err := m.store.Instance(ctx, userID)
	if err != nil || !ok {
		return err
	}
	err = m.ownerTx(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update agentsview.raw_devices set revoked_at = now() where revoked_at is null`)
		return err
	})
	// A start interrupted before its password or database existed has enrolled no devices.
	if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) && (pgErr.Code == "28P01" || pgErr.Code == "3D000") {
		return nil
	}
	return err
}

// Purge stops the User's Instance and removes its database, roles and data directory.
// Every step tolerates already being done, so a failed Delete can be repeated.
func (m *Manager) Purge(ctx context.Context, userID string) error {
	m.Stop(userID)
	n := namesFor(userID)
	admin, err := pgx.Connect(ctx, m.dbURL(m.adminUser, m.adminPassword, "postgres"))
	if err != nil {
		return fmt.Errorf("connect as platform role: %w", err)
	}
	defer func() { _ = admin.Close(context.WithoutCancel(ctx)) }()
	var dbExists bool
	if err := admin.QueryRow(ctx, `select exists (select from pg_database where datname = $1)`, n.db).Scan(&dbExists); err != nil {
		return err
	}
	if dbExists {
		// The platform role has SET but not INHERIT on the owner (see provision), so it
		// must become the owner to drop the database.
		for _, q := range []string{
			"SET ROLE " + ident(n.owner),
			"DROP DATABASE " + ident(n.db) + " WITH (FORCE)",
			"RESET ROLE",
		} {
			if _, err := admin.Exec(ctx, q); err != nil {
				return err
			}
		}
	}
	for _, role := range []string{n.rt, n.owner} {
		if _, err := admin.Exec(ctx, "DROP ROLE IF EXISTS "+ident(role)); err != nil {
			return err
		}
	}
	return os.RemoveAll(filepath.Join(m.dataDir, "instances", userID))
}
