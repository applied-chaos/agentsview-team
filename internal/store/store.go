// Package store is the Platform's Postgres access: migrations, Users and refresh tokens.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	// ErrInvalidToken covers unknown, expired and revoked refresh tokens and Disabled Users.
	ErrInvalidToken = errors.New("invalid refresh token")
	// ErrTokenReuse means a rotated-out token came back after RefreshGrace; its chain is now revoked.
	ErrTokenReuse = errors.New("refresh token reused")
)

// RefreshGrace lets a rotated-out token refresh again briefly: tabs and parallel requests share one cookie.
const RefreshGrace = 30 * time.Second

type User struct {
	ID       string
	Email    string
	Name     string
	IsAdmin  bool
	Disabled bool
}

type Store struct{ pool *pgxpool.Pool }

// Open connects, retrying while Postgres starts, and applies migrations.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err = pool.Ping(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("postgres: %w", err)
		}
		time.Sleep(time.Second)
	}
	s := &Store{pool: pool}
	return s, s.migrate(ctx)
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `select pg_advisory_lock(727001)`); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `select pg_advisory_unlock(727001)`) }()
	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (name text primary key, applied_at timestamptz not null default now())`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		var done bool
		if err := conn.QueryRow(ctx, `select exists (select 1 from schema_migrations where name = $1)`, f).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sqlText, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s: %w", f, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations (name) values ($1)`, f); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

const userCols = `id::text, email, name, is_admin, disabled`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.IsAdmin, &u.Disabled)
	return u, err
}

// UpsertUser returns the User with this email, creating it on first sight (as an
// Admin when a grant is pending) and stamping the sign-in.
func (s *Store) UpsertUser(ctx context.Context, email, name string) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		with p as (delete from pending_admins where email = $1 returning 1)
		insert into users (email, name, is_admin, last_sign_in_at) values ($1, $2, exists (select 1 from p), now())
		on conflict (email) do update set last_sign_in_at = now()
		returning `+userCols, email, name))
}

func (s *Store) UserByID(ctx context.Context, id string) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `select `+userCols+` from users where id = $1::uuid`, id))
}

// NewRefreshChain stores the first token of a new chain.
func (s *Store) NewRefreshChain(ctx context.Context, userID string, hash []byte, now, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into refresh_tokens (user_id, family_id, token_hash, expires_at, created_at)
		values ($1::uuid, gen_random_uuid(), $2, $3, $4)`, userID, hash, expires, now)
	return err
}

// RotateRefresh spends the presented token and stores its successor in the same chain.
// Reuse of a spent token revokes the whole chain. A Disabled or deleted User gets nothing.
func (s *Store) RotateRefresh(ctx context.Context, oldHash, newHash []byte, now, expires time.Time) (User, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		family          string
		userID          string
		expiresAt       time.Time
		usedAt, revoked *time.Time
		impID           *string
		impStarted      *time.Time
	)
	err = tx.QueryRow(ctx, `
		select family_id::text, user_id::text, expires_at, used_at, revoked_at, imp_user_id::text, imp_started_at
		from refresh_tokens where token_hash = $1 for update`, oldHash).
		Scan(&family, &userID, &expiresAt, &usedAt, &revoked, &impID, &impStarted)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrInvalidToken
	}
	if err != nil {
		return User{}, "", err
	}
	revokeFamily := func(why string) error {
		if err := endImpersonations(ctx, tx, now, why, `t.family_id = $1::uuid`, family); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `update refresh_tokens set revoked_at = $2 where family_id = $1::uuid and revoked_at is null`, family, now)
		return err
	}
	if revoked != nil || (usedAt != nil && now.Sub(*usedAt) >= RefreshGrace) {
		if err := revokeFamily("The refresh token was reused."); err != nil {
			return User{}, "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return User{}, "", err
		}
		return User{}, "", ErrTokenReuse
	}
	user, err := scanUser(tx.QueryRow(ctx, `select `+userCols+` from users where id = $1::uuid`, userID))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", err
	}
	if err != nil || user.Disabled || !expiresAt.After(now) {
		why := "The session expired."
		if err != nil || user.Disabled {
			why = "The Admin was disabled or deleted."
		}
		if err := revokeFamily(why); err != nil {
			return User{}, "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return User{}, "", err
		}
		return User{}, "", ErrInvalidToken
	}
	if _, err := tx.Exec(ctx, `update refresh_tokens set used_at = coalesce(used_at, $2) where token_hash = $1`, oldHash, now); err != nil {
		return User{}, "", err
	}
	if _, err := tx.Exec(ctx, `
		insert into refresh_tokens (user_id, family_id, token_hash, expires_at, created_at, imp_user_id, imp_started_at)
		values ($1::uuid, $2::uuid, $3, $4, $5, $6::uuid, $7)`, userID, family, newHash, expires, now, impID, impStarted); err != nil {
		return User{}, "", err
	}
	imp := ""
	if impID != nil {
		imp = *impID
	}
	return user, imp, tx.Commit(ctx)
}

// RevokeChain ends the chain a token belongs to; unknown tokens are ignored.
func (s *Store) RevokeChain(ctx context.Context, hash []byte, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		update refresh_tokens set revoked_at = $2
		where revoked_at is null and family_id = (select family_id from refresh_tokens where token_hash = $1)`, hash, now)
	return err
}
