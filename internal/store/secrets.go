package store

import "context"

// sealedColumns are every column holding a value sealed with PLATFORM_SECRET_KEY; label is the
// associated data each was sealed with.
var sealedColumns = []struct {
	table, key, col string
	label           func(key, col string) string
}{
	{"instances", "user_id", "owner_password", userLabel}, {"instances", "user_id", "runtime_password", userLabel},
	{"instances", "user_id", "bearer_token", userLabel}, {"instances", "user_id", "cursor_secret", userLabel},
	{"admin_instance", "id", "owner_password", userLabel}, {"admin_instance", "id", "runtime_password", userLabel},
	{"admin_instance", "id", "bearer_token", userLabel}, {"admin_instance", "id", "cursor_secret", userLabel},
	{"replay_devices", "host_id", "credential", func(key, _ string) string { return "replay/" + key + "/credential" }},
}

func userLabel(key, col string) string { return key + "/" + col }

// ResealSecrets replaces every sealed value with reseal(label, sealed) in one transaction, so a
// failure changes nothing. It returns how many values reseal changed.
func (s *Store) ResealSecrets(ctx context.Context, reseal func(label string, sealed []byte) ([]byte, error)) (changed int, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, c := range sealedColumns {
		rows, err := tx.Query(ctx, `select `+c.key+`::text, `+c.col+` from `+c.table+` for update`)
		if err != nil {
			return 0, err
		}
		type row struct {
			key    string
			sealed []byte
		}
		var all []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.key, &r.sealed); err != nil {
				rows.Close()
				return 0, err
			}
			all = append(all, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
		for _, r := range all {
			out, err := reseal(c.label(r.key, c.col), r.sealed)
			if err != nil {
				return 0, err
			}
			if string(out) == string(r.sealed) {
				continue
			}
			if _, err := tx.Exec(ctx, `update `+c.table+` set `+c.col+` = $2 where `+c.key+`::text = $1`, r.key, out); err != nil {
				return 0, err
			}
			changed++
		}
	}
	return changed, tx.Commit(ctx)
}
