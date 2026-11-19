package store

import (
	"context"
	"fmt"
	"time"
)

// StartImpersonation records Admin acting as target on the session the refresh token belongs to,
// and audits it, atomically.
func (s *Store) StartImpersonation(ctx context.Context, refreshHash []byte, by Actor, target User, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		update refresh_tokens set imp_user_id = $2::uuid, imp_started_at = $3
		where revoked_at is null and imp_user_id is null
		and family_id = (select family_id from refresh_tokens where token_hash = $1)`, refreshHash, target.ID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidToken
	}
	e := AuditEntry{Actor: by, Action: ActionImpersonationStarted, TargetID: &target.ID, TargetLabel: target.Name}
	if err := recordAudit(ctx, tx, now, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EndImpersonation stops any Impersonation on the session and audits the end. It is a no-op
// when the session is not impersonating.
func (s *Store) EndImpersonation(ctx context.Context, refreshHash []byte, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cond := `t.family_id = (select family_id from refresh_tokens where token_hash = $1)`
	if err := endImpersonations(ctx, tx, now, "", cond, refreshHash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EndExpiredImpersonations audits the end of Impersonations whose session ran out without
// anyone refreshing it.
func (s *Store) EndExpiredImpersonations(ctx context.Context, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := endImpersonations(ctx, tx, now, "The session expired.", `t.expires_at <= $1`, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lasted(d time.Duration) string {
	if d < time.Minute {
		return "Lasted under a minute"
	}
	return fmt.Sprintf("Lasted %d min", int(d.Minutes()))
}
