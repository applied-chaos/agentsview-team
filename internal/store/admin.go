package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrLastAdmin means the change would leave the Platform with no enabled Admin.
	ErrLastAdmin = errors.New("cannot remove the last Admin")
	// ErrNotFound means the User does not exist.
	ErrNotFound = pgx.ErrNoRows
)

const lockAdmins = 727002

type UserRow struct {
	User
	PendingAdmin bool
	CreatedAt    time.Time
	LastSignIn   *time.Time
}

// GrantAdmin makes an existing User an Admin now, or records a pending grant for
// an email that has not signed in. Repeating a grant changes and records nothing.
func (s *Store) GrantAdmin(ctx context.Context, email string, by Actor, now time.Time) (pending bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, lockAdmins); err != nil {
		return false, err
	}
	var id string
	var isAdmin bool
	err = tx.QueryRow(ctx, `select id::text, is_admin from users where email = $1`, email).Scan(&id, &isAdmin)
	entry := AuditEntry{Actor: by, Action: ActionAdminGranted, TargetLabel: email}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		tag, err := tx.Exec(ctx, `insert into pending_admins (email, created_at) values ($1, $2) on conflict do nothing`, email, now)
		if err != nil {
			return true, err
		}
		if tag.RowsAffected() == 1 {
			entry.Details = "Pending until first sign-in"
			if err := recordAudit(ctx, tx, now, entry); err != nil {
				return true, err
			}
		}
		return true, tx.Commit(ctx)
	case err != nil:
		return false, err
	case !isAdmin:
		if _, err := tx.Exec(ctx, `update users set is_admin = true where id = $1::uuid`, id); err != nil {
			return false, err
		}
		entry.TargetID = &id
		if err := recordAudit(ctx, tx, now, entry); err != nil {
			return false, err
		}
	}
	return false, tx.Commit(ctx)
}

// RevokeAdmin removes Admin from a User, or drops a pending grant. It refuses to
// remove the last enabled Admin, except from the CLI. Revoking someone who is not an Admin is a no-op.
func (s *Store) RevokeAdmin(ctx context.Context, email string, by Actor, now time.Time) error {
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, lockAdmins); err != nil {
		return err
	}
	entry := AuditEntry{Actor: by, Action: ActionAdminRevoked, TargetLabel: email}
	tag, err := tx.Exec(ctx, `delete from pending_admins where email = $1`, email)
	if err != nil {
		return err
	}
	changed := tag.RowsAffected() == 1
	if changed {
		entry.Details = "Pending grant withdrawn"
	}

	var id string
	var disabled bool
	err = tx.QueryRow(ctx, `select id::text, disabled from users where email = $1 and is_admin`, email).Scan(&id, &disabled)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return err
	default:
		var others int
		if err := tx.QueryRow(ctx, `select count(*) from users where is_admin and not disabled and id <> $1::uuid`, id).Scan(&others); err != nil {
			return err
		}
		if others == 0 && !disabled && !by.IsCLI() {
			return ErrLastAdmin
		}
		if _, err := tx.Exec(ctx, `update users set is_admin = false where id = $1::uuid`, id); err != nil {
			return err
		}
		entry.TargetID = &id
		changed = true
	}
	if changed {
		if err := recordAudit(ctx, tx, now, entry); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// PendingAdmin is an email granted Admin before its first sign-in; it has no User yet.
type PendingAdmin struct {
	Email     string
	CreatedAt time.Time
}

func (s *Store) ListUsers(ctx context.Context) ([]UserRow, []PendingAdmin, error) {
	rows, err := s.pool.Query(ctx, `
		select `+userCols+`, exists (select 1 from pending_admins p where p.email = users.email), created_at, last_sign_in_at
		from users order by name`)
	if err != nil {
		return nil, nil, err
	}
	users, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (u UserRow, err error) {
		err = r.Scan(&u.ID, &u.Email, &u.Name, &u.IsAdmin, &u.Disabled, &u.PendingAdmin, &u.CreatedAt, &u.LastSignIn)
		return
	})
	if err != nil {
		return nil, nil, err
	}
	rows, err = s.pool.Query(ctx, `
		select email, created_at from pending_admins where email not in (select email from users) order by email`)
	if err != nil {
		return nil, nil, err
	}
	pending, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (p PendingAdmin, err error) {
		err = r.Scan(&p.Email, &p.CreatedAt)
		return
	})
	return users, pending, err
}

func (s *Store) UserRowByID(ctx context.Context, id string) (UserRow, error) {
	var u UserRow
	err := s.pool.QueryRow(ctx, `select `+userCols+`, false, created_at, last_sign_in_at from users where id = $1::uuid`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.IsAdmin, &u.Disabled, &u.PendingAdmin, &u.CreatedAt, &u.LastSignIn)
	return u, err
}
