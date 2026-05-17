package control

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	ckconfig "github.com/openclaw/crawlkit/config"
	ckstore "github.com/openclaw/crawlkit/store"
	"github.com/uinaf/fincrawl/internal/config"
	"github.com/uinaf/fincrawl/internal/store"
)

func TestStatusReportsPrivacySafeSyncState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "archive.db")
	if err := store.SaveSyncState(ctx, dbPath, store.SyncState{
		ID:                store.IntercomTailSyncStateID,
		Provider:          store.ProviderIntercom,
		CursorKind:        "updated_at",
		HighWaterMark:     "2026-05-16T10:00:00Z",
		ActiveWindowStart: "2026-05-16T10:00:00Z",
		ActiveWindowEnd:   "2026-05-16T11:00:00Z",
		LastProviderID:    "conv_secret_like_fake",
		PageCursor:        "cursor_secret_like_fake",
	}); err != nil {
		t.Fatal(err)
	}
	report := Status(ctx, config.Runtime{
		Paths:  ckconfig.Paths{ConfigPath: filepath.Join(root, "config.toml")},
		Config: ckconfig.RuntimeConfig{DBPath: dbPath},
	})
	if report.State != "ready" {
		t.Fatalf("state = %q", report.State)
	}
	if len(report.SyncStates) != 1 {
		t.Fatalf("sync states = %#v", report.SyncStates)
	}
	state := report.SyncStates[0]
	if state.State != "active" || !state.ResumeAvailable {
		t.Fatalf("sync state = %#v", state)
	}
	if !state.HasLastProviderID || !state.HasPageCursor {
		t.Fatalf("marker booleans not set: %#v", state)
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "conv_secret_like_fake") || strings.Contains(string(body), "cursor_secret_like_fake") {
		t.Fatalf("status leaked provider marker: %s", string(body))
	}
}

func TestStatusReportsEntityCounts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "archive.db")
	fixture, err := store.LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	report := Status(ctx, config.Runtime{
		Paths:  ckconfig.Paths{ConfigPath: filepath.Join(root, "config.toml")},
		Config: ckconfig.RuntimeConfig{DBPath: dbPath},
	})
	counts := map[string]int64{}
	for _, count := range report.Counts {
		counts[count.ID] = count.Value
	}
	if counts["admins"] != 2 || counts["teams"] != 1 || counts["tags"] != 2 || counts["contacts"] != 2 {
		t.Fatalf("entity counts = %#v", counts)
	}
}

func TestStatusReadsPreEntityArchives(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: legacyStatusSchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversations(
		id, workspace_id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
	) values('conv_1', 'workspace_1', 'intercom', 'ic_1', 'Synthetic subject', 'open', '', '', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	report := Status(ctx, config.Runtime{
		Paths:  ckconfig.Paths{ConfigPath: filepath.Join(root, "config.toml")},
		Config: ckconfig.RuntimeConfig{DBPath: dbPath},
	})
	if report.State != "ready" {
		t.Fatalf("state = %q warnings = %#v", report.State, report.Warnings)
	}
	counts := map[string]int64{}
	for _, count := range report.Counts {
		counts[count.ID] = count.Value
	}
	if counts["conversations"] != 1 || counts["admins"] != 0 {
		t.Fatalf("counts = %#v", counts)
	}
}

const legacyStatusSchema = `
create table if not exists conversations (
	id text primary key,
	workspace_id text not null,
	provider text not null,
	provider_id text not null,
	subject text not null,
	state text not null,
	assignee text not null,
	rating text not null,
	fin_status text not null,
	created_at text not null,
	updated_at text not null
);

create table if not exists conversation_parts (
	id text primary key,
	conversation_id text not null references conversations(id) on delete cascade,
	provider text not null,
	provider_id text not null,
	part_type text not null,
	author_name text not null,
	body text not null,
	created_at text not null,
	updated_at text not null
);

create table if not exists raw_blobs (
	hash text primary key,
	provider text not null,
	record_type text not null,
	provider_id text not null,
	json text not null,
	created_at text not null
);

create table if not exists sync_state (
	id text primary key,
	provider text not null,
	cursor_kind text not null,
	high_water_mark text not null,
	active_window_start text not null default '',
	active_window_end text not null default '',
	last_provider_id text not null default '',
	page_cursor text not null default '',
	updated_at text not null
);
`
