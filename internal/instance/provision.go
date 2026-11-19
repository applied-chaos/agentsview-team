package instance

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/applied-chaos/agentsview-team/internal/store"
	"github.com/applied-chaos/agentsview-team/internal/tokens"
)

// Runtime grants, from upstream's hostedProjectionPrivileges and custody lists
// (internal/postgres/raw_runtime_privileges.go, raw_ingest_schema.go) for the
// pinned agentsview version. Re-check on every upgrade.
const runtimeGrants = `
SET search_path = agentsview;
GRANT USAGE ON SCHEMA agentsview TO %[1]s;
GRANT SELECT ON ALL TABLES IN SCHEMA agentsview TO %[1]s;
GRANT UPDATE (dialogue_text) ON messages TO %[1]s;
GRANT UPDATE (observed_outcome, observed_repeat, sequence_ending) ON tool_calls TO %[1]s;
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions, pinned_messages, raw_message_dialogue, session_sources,
  raw_curation, raw_pins, raw_upload_sessions TO %[1]s;
GRANT SELECT, INSERT ON messages, tool_calls, tool_result_events, usage_events, session_plan_usage,
  secret_findings, raw_projection_generations, raw_source_contributions, raw_session_public_aliases,
  raw_embedding_outbox, raw_device_tokens, raw_manifests, raw_manifest_entries, raw_manifest_objects TO %[1]s;
GRANT SELECT, INSERT, DELETE ON friction_findings, starred_sessions, raw_session_links TO %[1]s;
GRANT SELECT, INSERT, UPDATE ON excluded_sessions, raw_source_projections, raw_session_groups,
  raw_content_revisions, raw_session_branches, raw_corpus_state, raw_projection_rollouts,
  raw_objects, raw_source_heads, raw_ingest_jobs TO %[1]s;
GRANT USAGE ON SEQUENCE raw_ingest_jobs_id_seq, tool_calls_id_seq, tool_result_events_id_seq,
  usage_events_id_seq, pinned_messages_id_seq TO %[1]s;
`

// configKind says whether config.toml carries the owner login; serving never does.
type configKind bool

const (
	withOwnerCredentials configKind = true
	servingOnly          configKind = false
)

type names struct{ db, owner, rt string }

func namesFor(userID string) names {
	db := "av_" + strings.ReplaceAll(userID, "-", "")
	return names{db: db, owner: db + "_owner", rt: db + "_rt"}
}

type secretSet struct{ ownerPW, runtimePW, token, cursor string }

// loadSecrets returns the User's secrets, creating and sealing them on first use.
func (m *Manager) loadSecrets(ctx context.Context, userID string) (secretSet, error) {
	row, ok, err := m.store.Instance(ctx, userID)
	if err != nil {
		return secretSet{}, err
	}
	if !ok {
		var s secretSet
		for _, dst := range []*string{&s.ownerPW, &s.runtimePW, &s.token} {
			if *dst, err = tokens.Random(24); err != nil {
				return secretSet{}, err
			}
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return secretSet{}, err
		}
		s.cursor = base64.StdEncoding.EncodeToString(raw)
		row = store.InstanceRow{UserID: userID, DBName: namesFor(userID).db}
		for _, f := range []struct {
			dst   *[]byte
			label string
			val   string
		}{
			{&row.OwnerPassword, "owner_password", s.ownerPW}, {&row.RuntimePassword, "runtime_password", s.runtimePW},
			{&row.BearerToken, "bearer_token", s.token}, {&row.CursorSecret, "cursor_secret", s.cursor},
		} {
			if *f.dst, err = m.box.Seal(userID+"/"+f.label, []byte(f.val)); err != nil {
				return secretSet{}, err
			}
		}
		create := m.store.CreateInstance
		if userID == m.adminID {
			create = m.store.CreateAdminInstance
		}
		if err := create(ctx, row); err != nil {
			return secretSet{}, err
		}
		// A concurrent first run may have won; read back whichever row is stored.
		if row, _, err = m.store.Instance(ctx, userID); err != nil {
			return secretSet{}, err
		}
	}
	var s secretSet
	for _, f := range []struct {
		dst    *string
		label  string
		sealed []byte
	}{
		{&s.ownerPW, "owner_password", row.OwnerPassword}, {&s.runtimePW, "runtime_password", row.RuntimePassword},
		{&s.token, "bearer_token", row.BearerToken}, {&s.cursor, "cursor_secret", row.CursorSecret},
	} {
		plain, err := m.box.Open(userID+"/"+f.label, f.sealed)
		if err != nil {
			return secretSet{}, fmt.Errorf("open %s: %w", f.label, err)
		}
		*f.dst = string(plain)
	}
	return s, nil
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// initAdmin loads the Admin Instance's tenant id, creating it with its secrets on first boot.
func (m *Manager) initAdmin(ctx context.Context) error {
	id, ok, err := m.store.AdminInstance(ctx)
	if err != nil {
		return err
	}
	if !ok {
		if id, err = newUUID(); err != nil {
			return err
		}
		m.adminID = id
		_, err = m.loadSecrets(ctx, id)
		return err
	}
	m.adminID = id
	return nil
}

func (m *Manager) dbURL(user, password, db string) string {
	u := *m.pgBase
	u.User = url.UserPassword(user, password)
	u.Path = "/" + db
	return u.String()
}

func ident(s string) string { return pgx.Identifier{s}.Sanitize() }
func lit(s string) string   { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// provision creates or repairs everything an Instance needs. Every step is a
// no-op when already done, so it runs again on each start.
func (m *Manager) provision(ctx context.Context, userID string) (secretSet, string, error) {
	sec, err := m.loadSecrets(ctx, userID)
	if err != nil {
		return sec, "", err
	}
	n := namesFor(userID)
	dir := filepath.Join(m.dataDir, "instances", userID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return sec, "", err
	}

	admin, err := pgx.Connect(ctx, m.dbURL(m.adminUser, m.adminPassword, "postgres"))
	if err != nil {
		return sec, "", fmt.Errorf("connect as platform role: %w", err)
	}
	defer func() { _ = admin.Close(context.WithoutCancel(ctx)) }()
	var version int
	if err := admin.QueryRow(ctx, `select current_setting('server_version_num')::int`).Scan(&version); err != nil {
		return sec, "", err
	}
	grantOwner := "GRANT " + ident(n.owner) + " TO " + ident(m.adminUser)
	if version >= 160000 {
		// PG16+: CREATE DATABASE ... OWNER needs SET on the owner; the creator only gets ADMIN.
		grantOwner += " WITH INHERIT FALSE, SET TRUE"
	}
	for _, r := range []struct{ role, opts string }{
		{n.owner, "LOGIN"},
		{n.rt, "LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT"},
	} {
		var exists bool
		if err := admin.QueryRow(ctx, `select exists (select from pg_roles where rolname = $1)`, r.role).Scan(&exists); err != nil {
			return sec, "", err
		}
		if !exists {
			if _, err := admin.Exec(ctx, "CREATE ROLE "+ident(r.role)+" "+r.opts); err != nil {
				return sec, "", err
			}
		}
	}
	for _, q := range []string{
		"ALTER ROLE " + ident(n.owner) + " PASSWORD " + lit(sec.ownerPW),
		"ALTER ROLE " + ident(n.rt) + " PASSWORD " + lit(sec.runtimePW),
		grantOwner,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			return sec, "", err
		}
	}
	var dbExists bool
	if err := admin.QueryRow(ctx, `select exists (select from pg_database where datname = $1)`, n.db).Scan(&dbExists); err != nil {
		return sec, "", err
	}
	if !dbExists {
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident(n.db)+" OWNER "+ident(n.owner)); err != nil {
			return sec, "", err
		}
	}

	ownerURL := m.dbURL(n.owner, sec.ownerPW, n.db)
	owner, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		return sec, "", fmt.Errorf("connect as owner: %w", err)
	}
	defer func() { _ = owner.Close(context.WithoutCancel(ctx)) }()
	if _, err := owner.Exec(ctx, fmt.Sprintf(`
		REVOKE ALL ON DATABASE %[1]s FROM PUBLIC;
		GRANT CONNECT ON DATABASE %[1]s TO %[2]s, %[3]s;
		CREATE SCHEMA IF NOT EXISTS agentsview;`, ident(n.db), ident(n.rt), ident(m.adminUser))); err != nil {
		return sec, "", err
	}

	if err := m.writeConfig(dir, userID, n, sec, withOwnerCredentials); err != nil {
		return sec, "", err
	}
	cmd := exec.CommandContext(ctx, m.bin, "replica", "pg", "hosted-provision", "provision")
	cmd.Env = m.childEnv(dir, "")
	if out, err := cmd.CombinedOutput(); err != nil {
		return sec, "", fmt.Errorf("hosted-provision: %w: %s", err, tail(redact(string(out), sec)))
	}
	if _, err := owner.Exec(ctx, fmt.Sprintf(runtimeGrants, ident(n.rt))); err != nil {
		return sec, "", fmt.Errorf("runtime grants: %w", err)
	}
	if err := m.writeConfig(dir, userID, n, sec, servingOnly); err != nil {
		return sec, "", err
	}
	return sec, dir, nil
}

func (m *Manager) writeConfig(dir, userID string, n names, sec secretSet, creds configKind) error {
	insecure := m.pgBase.Query().Get("sslmode") == "disable"
	var b strings.Builder
	fmt.Fprintf(&b, "default_pg = \"hosted\"\nrequire_auth = true\ncursor_secret = %q\n\n", sec.cursor)
	if creds == withOwnerCredentials {
		fmt.Fprintf(&b, "[pg.provision]\nurl = %q\nschema = \"agentsview\"\nraw_tenant = %q\nallow_insecure = %t\n\n",
			m.dbURL(n.owner, sec.ownerPW, n.db), userID, insecure)
	}
	// ADR 0003: one parse job per poll tick, so the poll interval bounds backfill speed.
	fmt.Fprintf(&b, "[pg.hosted]\nurl = %q\nschema = \"agentsview\"\nraw_tenant = %q\nraw_derivation = true\nraw_poll_seconds = 1\nallow_insecure = %t\n",
		m.dbURL(n.rt, sec.runtimePW, n.db), userID, insecure)
	return writeFileAtomic(filepath.Join(dir, "config.toml"), []byte(b.String()))
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = s[len(s)-400:]
	}
	return s
}

func redact(s string, sec secretSet) string {
	for _, v := range []string{sec.ownerPW, sec.runtimePW, sec.token, sec.cursor} {
		s = strings.ReplaceAll(s, v, "***")
	}
	return s
}
