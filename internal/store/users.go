package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// SystemActor is the Platform acting on its own, such as an auto-Disable at sign-in.
var SystemActor = Actor{Name: "Platform"}

// DisableUser blocks sign-in, deletes the User's refresh tokens and revokes their live
// Hosts. Repeating it changes and audits nothing. ErrNotFound when there is no such User.
func (s *Store) DisableUser(ctx context.Context, id string, by Actor, details string, now time.Time) error {
	return s.disableUser(ctx, id, by, details, now, true)
}

// BlockUser is DisableUser without the audit entry, for the first step of a Delete (which audits itself).
func (s *Store) BlockUser(ctx context.Context, id string, now time.Time) error {
	return s.disableUser(ctx, id, SystemActor, "", now, false)
}

func (s *Store) disableUser(ctx context.Context, id string, by Actor, details string, now time.Time, audit bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var email string
	var wasDisabled bool
	if err := tx.QueryRow(ctx, `select email, disabled from users where id = $1::uuid for update`, id).Scan(&email, &wasDisabled); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update users set disabled = true where id = $1::uuid`, id); err != nil {
		return err
	}
	if err := endImpersonations(ctx, tx, now, "The Admin was disabled.", `t.user_id = $1::uuid`, id); err != nil {
		return err
	}
	if err := endImpersonations(ctx, tx, now, "The impersonated User was disabled.", `t.imp_user_id = $1::uuid`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from refresh_tokens where user_id = $1::uuid`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update hosts set revoked_at = $2 where user_id = $1::uuid and revoked_at is null`, id, now); err != nil {
		return err
	}
	if audit && !wasDisabled {
		e := AuditEntry{Actor: by, Action: ActionUserDisabled, TargetID: &id, TargetLabel: email, Details: details}
		if err := recordAudit(ctx, tx, now, e); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DisableUserByEmail is DisableUser for the sign-in rule; id is empty when the email has no User.
func (s *Store) DisableUserByEmail(ctx context.Context, email string, now time.Time) (id string, err error) {
	err = s.pool.QueryRow(ctx, `select id::text from users where email = $1`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, s.DisableUser(ctx, id, SystemActor, "No longer matches the sign-in rule", now)
}

// EnableUser reverses a Disable. Revoked Hosts stay revoked.
func (s *Store) EnableUser(ctx context.Context, id string, by Actor, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var email string
	var wasDisabled bool
	if err := tx.QueryRow(ctx, `select email, disabled from users where id = $1::uuid for update`, id).Scan(&email, &wasDisabled); err != nil {
		return err
	}
	if wasDisabled {
		if _, err := tx.Exec(ctx, `update users set disabled = false where id = $1::uuid`, id); err != nil {
			return err
		}
		e := AuditEntry{Actor: by, Action: ActionUserEnabled, TargetID: &id, TargetLabel: email}
		if err := recordAudit(ctx, tx, now, e); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DeleteUser removes the User row (their tokens, Hosts and Instance row cascade) and audits it.
func (s *Store) DeleteUser(ctx context.Context, id string, by Actor, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var email string
	if err := tx.QueryRow(ctx, `delete from users where id = $1::uuid returning email`, id).Scan(&email); err != nil {
		return err
	}
	e := AuditEntry{Actor: by, Action: ActionUserDeleted, TargetID: &id, TargetLabel: email, Details: "Instance removed"}
	if err := recordAudit(ctx, tx, now, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
