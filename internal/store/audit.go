package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Category string

const (
	CategoryAdminGrants   Category = "admin_grants"
	CategoryUserChanges   Category = "user_changes"
	CategoryImpersonation Category = "impersonation"
	CategoryInstanceView  Category = "instance_view"
	CategoryHosts         Category = "hosts"
	CategoryInstallTokens Category = "install_tokens"
)

// categories are the Audit page's type filters, including ones no Action uses yet.
var categories = []Category{CategoryImpersonation, CategoryInstanceView, CategoryAdminGrants, CategoryUserChanges, CategoryHosts, CategoryInstallTokens}

type Action string

const (
	ActionAdminGranted  Action = "admin_granted"
	ActionAdminRevoked  Action = "admin_revoked"
	ActionUserDisabled  Action = "user_disabled"
	ActionUserEnabled   Action = "user_enabled"
	ActionUserDeleted   Action = "user_deleted"
	ActionAdminInstance Action = "admin_instance_opened"

	ActionImpersonationStarted   Action = "impersonation_started"
	ActionImpersonationEnded     Action = "impersonation_ended"
	ActionImpersonatedRetry      Action = "impersonated_instance_retry"
	ActionImpersonatedHostRename Action = "impersonated_host_rename"
	ActionImpersonatedHostRevoke Action = "impersonated_host_revoke"
	ActionImpersonatedHostDelete Action = "impersonated_host_delete"
	ActionImpersonatedTokenNew   Action = "impersonated_token_create"
	ActionImpersonatedTokenStop  Action = "impersonated_token_revoke"
	ActionImpersonatedWrite      Action = "impersonated_instance_write"
)

// actions is the one table of what each Action is: its category and the label Admins read.
var actions = map[Action]struct {
	Category Category
	Label    string
}{
	ActionAdminGranted:           {CategoryAdminGrants, "Admin granted"},
	ActionAdminRevoked:           {CategoryAdminGrants, "Admin role removed"},
	ActionUserDisabled:           {CategoryUserChanges, "User disabled"},
	ActionUserEnabled:            {CategoryUserChanges, "User enabled"},
	ActionUserDeleted:            {CategoryUserChanges, "User deleted"},
	ActionAdminInstance:          {CategoryInstanceView, "Admin Instance opened"},
	ActionImpersonationStarted:   {CategoryImpersonation, "Impersonation started"},
	ActionImpersonationEnded:     {CategoryImpersonation, "Impersonation ended"},
	ActionImpersonatedRetry:      {CategoryImpersonation, "Instance restarted while impersonating"},
	ActionImpersonatedHostRename: {CategoryImpersonation, "Host renamed while impersonating"},
	ActionImpersonatedHostRevoke: {CategoryImpersonation, "Host revoked while impersonating"},
	ActionImpersonatedHostDelete: {CategoryImpersonation, "Host deleted while impersonating"},
	ActionImpersonatedTokenNew:   {CategoryImpersonation, "Install Token created while impersonating"},
	ActionImpersonatedTokenStop:  {CategoryImpersonation, "Install Token revoked while impersonating"},
	ActionImpersonatedWrite:      {CategoryImpersonation, "Instance changed while impersonating"},
}

func (a Action) Category() Category { return actions[a].Category }

// Label is the Admin-facing name; an unknown stored action shows as itself.
func (a Action) Label() string {
	if l := actions[a].Label; l != "" {
		return l
	}
	return string(a)
}

func ParseCategory(s string) (Category, bool) {
	for _, c := range categories {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

type Actor struct {
	ID   *string
	Name string
}

var CLIActor = Actor{Name: "CLI"}

// IsCLI is true for the command line, which is how an Admin recovers from a mistake.
func (a Actor) IsCLI() bool { return a.ID == nil && a.Name == CLIActor.Name }

type AuditEntry struct {
	Actor       Actor
	Action      Action
	TargetID    *string
	TargetLabel string
	Details     string
}

type AuditRow struct {
	ID          int64
	At          time.Time
	ActorName   string
	Action      Action
	TargetLabel string
	Details     string
}

func recordAudit(ctx context.Context, tx pgx.Tx, now time.Time, e AuditEntry) error {
	_, err := tx.Exec(ctx, `
		insert into audit_log (at, actor_id, actor_name, action, category, target_id, target_label, details)
		values ($1, $2::uuid, $3, $4, $5, $6::uuid, $7, $8)`,
		now, e.Actor.ID, e.Actor.Name, string(e.Action), string(e.Action.Category()), e.TargetID, e.TargetLabel, e.Details)
	return err
}

// RecordAudit appends an entry for Admin actions that do not go through a store mutation.
func (s *Store) RecordAudit(ctx context.Context, now time.Time, e AuditEntry) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := recordAudit(ctx, tx, now, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type AuditQuery struct {
	Text       string
	Categories []Category // empty means all
	Since      *time.Time
	Limit      int
	Offset     int
}

func (s *Store) ListAudit(ctx context.Context, q AuditQuery) ([]AuditRow, int, error) {
	where := `where ($1 = '' or actor_name ilike '%'||$1||'%' or target_label ilike '%'||$1||'%' or details ilike '%'||$1||'%')
		and ($2::text[] is null or category = any($2))
		and ($3::timestamptz is null or at >= $3)`
	var cats []string
	for _, c := range q.Categories {
		cats = append(cats, string(c))
	}
	args := []any{escapeLike(q.Text), cats, q.Since}
	var total int
	if err := s.pool.QueryRow(ctx, `select count(*) from audit_log `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `select id, at, actor_name, action, target_label, details from audit_log `+where+
		` order by at desc, id desc limit $4 offset $5`, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditRow
	for rows.Next() {
		var r AuditRow
		if err := rows.Scan(&r.ID, &r.At, &r.ActorName, &r.Action, &r.TargetLabel, &r.Details); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// endImpersonations audits an Impersonation end for each live session in the refresh-token
// chains selected by cond (written against alias t), and clears Impersonation on those chains.
// Run it before the revoke or delete that makes the end happen, in the same transaction.
func endImpersonations(ctx context.Context, tx pgx.Tx, now time.Time, why, cond string, args ...any) error {
	rows, err := tx.Query(ctx, `
		select t.family_id::text, a.id::text, a.name, t.imp_user_id::text, t.imp_started_at,
			coalesce((select name from users where id = t.imp_user_id), 'Deleted User')
		from refresh_tokens t join users a on a.id = t.user_id
		where t.imp_user_id is not null and t.used_at is null and t.revoked_at is null and `+cond, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var families []string
	var entries []AuditEntry
	for rows.Next() {
		var family, admin, name, target, label string
		var started time.Time
		if err := rows.Scan(&family, &admin, &name, &target, &started, &label); err != nil {
			return err
		}
		families = append(families, family)
		entries = append(entries, AuditEntry{
			Actor: Actor{ID: &admin, Name: name}, Action: ActionImpersonationEnded,
			TargetID: &target, TargetLabel: label, Details: strings.TrimSpace(lasted(now.Sub(started)) + ". " + why),
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(families) == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, `update refresh_tokens set imp_user_id = null, imp_started_at = null where family_id = any($1::uuid[])`, families); err != nil {
		return err
	}
	for _, e := range entries {
		if err := recordAudit(ctx, tx, now, e); err != nil {
			return err
		}
	}
	return nil
}
