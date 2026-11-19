// Command spike-replay re-uploads every committed manifest of one Instance to another Instance as
// one mirrored device, using upstream's own raw-sync client. Built inside the upstream module.
//
// Env: SRC_PG_URL, SRC_TENANT, SRC_DATA_DIR, AGENTSVIEW_RAW_SYNC_URL,
// AGENTSVIEW_RAW_SYNC_DEVICE_ID, AGENTSVIEW_RAW_SYNC_CREDENTIAL.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.kenn.io/agentsview/internal/artifact"
	"go.kenn.io/agentsview/internal/rawclient"
	"go.kenn.io/agentsview/internal/rawsync"
)

func main() {
	ctx := context.Background()
	db, err := sql.Open("pgx", os.Getenv("SRC_PG_URL"))
	must(err)
	conn, err := db.Conn(ctx)
	must(err)
	_, err = conn.ExecContext(ctx, `SELECT set_config('agentsview.tenant_id', $1, false)`, os.Getenv("SRC_TENANT"))
	must(err)

	// Same call the Instance makes; fails with "vault is locked" while that Instance runs.
	repo, err := artifact.OpenRepository(ctx, filepath.Join(os.Getenv("SRC_DATA_DIR"), "raw-sync"))
	must(err)
	defer repo.Close()
	objects, err := rawsync.NewArtifactObjectStore(repo.Content())
	must(err)

	client, err := rawclient.NewClient(rawclient.Config{
		BaseURL:    os.Getenv("AGENTSVIEW_RAW_SYNC_URL"),
		DeviceID:   os.Getenv("AGENTSVIEW_RAW_SYNC_DEVICE_ID"),
		Credential: os.Getenv("AGENTSVIEW_RAW_SYNC_CREDENTIAL"),
	})
	must(err)

	rows, err := conn.QueryContext(ctx, `SELECT manifest_id, canonical_json FROM agentsview.raw_manifests ORDER BY accepted_at, generation`)
	must(err)
	type item struct {
		id   string
		json []byte
	}
	var items []item
	for rows.Next() {
		var it item
		must(rows.Scan(&it.id, &it.json))
		items = append(items, it)
	}
	must(rows.Err())

	// ponytail: parent receipts kept in memory, so a rerun must start against an empty target.
	heads := map[string]string{}
	uploaded, committed := 0, 0
	tenant := os.Getenv("SRC_TENANT")
	for _, it := range items {
		var m rawsync.Manifest
		must(json.Unmarshal(it.json, &m))
		key := fmt.Sprintf("%s\x00%s\x00%s", m.Provider, m.ConfiguredRootID, m.SourceKey)
		m.ExpectedParentReceipt = heads[key]

		var refs []rawsync.ObjectRef
		for _, e := range m.Entries {
			refs = append(refs, e.Objects...)
		}
		missing, err := client.MissingObjects(ctx, m.Provider, refs)
		must(err)
		for _, ref := range missing {
			var buf bytes.Buffer
			_, err := objects.CopyObject(ctx, tenant, ref, &buf)
			must(err)
			must(client.UploadObject(ctx, m.Provider, ref, bytes.NewReader(buf.Bytes())))
			uploaded++
		}
		res, err := client.CommitManifest(ctx, m)
		must(err)
		heads[key] = res.Receipt
		committed++
	}
	fmt.Printf("manifests=%d committed=%d objects_uploaded=%d\n", len(items), committed, uploaded)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
