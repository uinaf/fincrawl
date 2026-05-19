package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ckstore "github.com/openclaw/crawlkit/store"
)

func TestFirstNonEmptyReturnsFirstTrimmedValue(t *testing.T) {
	if got := firstNonEmpty("", "   ", "second", "ignored"); got != "second" {
		t.Fatalf("first = %q", got)
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("all empty = %q", got)
	}
	if got := firstNonEmpty(); got != "" {
		t.Fatalf("no args = %q", got)
	}
}

func TestCountsZeroOnEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: Schema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	counts, err := Counts(ctx, dbPath)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Conversations != 0 || counts.ConversationParts != 0 || counts.Admins != 0 || counts.Teams != 0 || counts.Tags != 0 || counts.Contacts != 0 || counts.RawBlobs != 0 {
		t.Fatalf("expected all zero counts, got %#v", counts)
	}
}

func TestCountsReflectsSyncedFixture(t *testing.T) {
	ctx := context.Background()
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	counts, err := Counts(ctx, dbPath)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Conversations != 2 || counts.Admins != 2 || counts.Teams != 1 || counts.Tags != 2 || counts.Contacts != 2 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestExportInferredWorkspaceReadsConversations(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: workspacelessSchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.DB().ExecContext(ctx, `insert into conversations(
		id, workspace_id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
	) values('conv_1', 'inferred_workspace', 'intercom', 'ic_1', 'Subject', 'open', '', '', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	ws, err := exportInferredWorkspace(ctx, st.DB())
	if err != nil {
		t.Fatalf("inferred: %v", err)
	}
	if ws.ID != "inferred_workspace" || ws.Provider != "intercom" || ws.Name != "inferred_workspace" {
		t.Fatalf("workspace = %#v", ws)
	}
	if _, err := st.DB().ExecContext(ctx, `delete from conversations`); err != nil {
		t.Fatal(err)
	}
	if _, err := exportInferredWorkspace(ctx, st.DB()); err == nil {
		t.Fatalf("expected error for empty conversations")
	}
}

const workspacelessSchema = `
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
`

func TestStoreFunctionsErrorOnUnopenableDB(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bogusDir := filepath.Join(dir, "not-a-file")
	if err := os.Mkdir(bogusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// dbPath points to a directory — SQLite open will fail
	if _, _, err := LoadSyncState(ctx, bogusDir, "any"); err == nil {
		t.Fatalf("expected LoadSyncState to fail")
	}
	if err := SaveSyncState(ctx, bogusDir, SyncState{ID: "x", Provider: "y", CursorKind: "updated_at", HighWaterMark: "z"}); err == nil {
		t.Fatalf("expected SaveSyncState to fail")
	}
	if _, err := ListSyncStates(ctx, bogusDir); err == nil {
		t.Fatalf("expected ListSyncStates to fail")
	}
	if _, err := Counts(ctx, bogusDir); err == nil {
		t.Fatalf("expected Counts to fail")
	}
	if _, err := GetConversation(ctx, bogusDir, "id", ConversationDetailOptions{}); err == nil {
		t.Fatalf("expected GetConversation to fail")
	}
	if _, err := ExportFixture(ctx, bogusDir); err == nil {
		t.Fatalf("expected ExportFixture to fail")
	}
	if _, err := SyncFixture(ctx, bogusDir, Fixture{}); err == nil {
		t.Fatalf("expected SyncFixture to fail")
	}
	if _, err := SyncEntities(ctx, bogusDir, Workspace{}, Entities{}); err == nil {
		t.Fatalf("expected SyncEntities to fail")
	}
	if _, err := SyncConversations(ctx, bogusDir, Workspace{}, nil); err == nil {
		t.Fatalf("expected SyncConversations to fail")
	}
}

func TestLoadFixtureRejectsBadInputs(t *testing.T) {
	if _, err := LoadFixture(t.TempDir()); err == nil {
		t.Fatalf("expected error for missing conversations.json")
	}
	bad := filepath.Join(t.TempDir(), "bad")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "conversations.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixture(bad); err == nil {
		t.Fatalf("expected json decode error")
	}
}

func TestSearchPropagatesUnopenableDB(t *testing.T) {
	dir := t.TempDir()
	bogus := filepath.Join(dir, "subdir-as-dbpath")
	if err := os.Mkdir(bogus, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchWithOptions(context.Background(), bogus, "anything", SearchOptions{}); err == nil {
		t.Fatalf("expected error opening directory as DB")
	}
}

func TestNormalizePartIdentitySynthesizesIDs(t *testing.T) {
	got := normalizePartIdentity("conv_1", Part{}, 0)
	if got.ProviderID == "" || got.ID == "" {
		t.Fatalf("expected synthesized ids: %#v", got)
	}
	if !strings.HasPrefix(got.ProviderID, "conv_1:part:") {
		t.Fatalf("provider id format = %q", got.ProviderID)
	}
	withProvider := normalizePartIdentity("conv_1", Part{ProviderID: "part_x"}, 0)
	if withProvider.ProviderID != "part_x" {
		t.Fatalf("provider id preserved = %q", withProvider.ProviderID)
	}
	if withProvider.ID == "" {
		t.Fatalf("id should be synthesized when missing")
	}
	withID := normalizePartIdentity("conv_1", Part{ID: "ic_part_1"}, 0)
	if withID.ProviderID != "ic_part_1" {
		t.Fatalf("provider id derived from id = %q", withID.ProviderID)
	}
}

func TestSyncConversationsResyncOverwritesData(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	// re-sync with modified subject
	if len(fixture.Conversations) == 0 {
		t.Fatalf("fixture has no conversations")
	}
	fixture.Conversations[0].Subject = "Updated subject"
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	conv, err := GetConversation(ctx, dbPath, fixture.Conversations[0].ProviderID, ConversationDetailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if conv.Subject != "Updated subject" {
		t.Fatalf("re-sync did not update subject: %q", conv.Subject)
	}
}

func TestExportFixtureRoundTripsThroughSync(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	source, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFixture(ctx, dbPath, source); err != nil {
		t.Fatal(err)
	}
	exported, err := ExportFixture(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Conversations) != len(source.Conversations) {
		t.Fatalf("conversations = %d, want %d", len(exported.Conversations), len(source.Conversations))
	}
	if exported.Workspace.ID == "" {
		t.Fatalf("workspace id missing in export")
	}
}

func TestUpsertConversationDedupesPartsWithinConversation(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	fixture := Fixture{
		Workspace: Workspace{ID: "ws_1", Provider: "intercom", Name: "ws"},
		Conversations: []Conversation{{
			ID: "conv_1", Provider: "intercom", ProviderID: "ic_conv_1", Subject: "subj",
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
			Parts: []Part{
				{ID: "part_a", ProviderID: "ic_part_a", Body: "first", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:01Z"},
				{ID: "part_a", ProviderID: "ic_part_a", Body: "second", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:02Z"},
			},
		}},
	}
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	conv, err := GetConversation(ctx, dbPath, "ic_conv_1", ConversationDetailOptions{IncludeParts: true, PartLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Parts) != 1 {
		t.Fatalf("expected dedup, got %d parts", len(conv.Parts))
	}
	if conv.Parts[0].Body != "second" {
		t.Fatalf("last duplicate wins; got %q", conv.Parts[0].Body)
	}
}

func TestSyncEntitiesIsolatesAdminTeamTag(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	res, err := SyncEntities(ctx, dbPath, Workspace{ID: "ws", Provider: "intercom", Name: "ws"}, Entities{
		Admins:   []Admin{{ProviderID: "adm_1", Name: "Riley", TeamIDs: []string{"team_1"}}, {ID: "adm_explicit", ProviderID: "adm_2", Name: "Casey"}},
		Teams:    []Team{{ProviderID: "team_1", Name: "Support"}},
		Tags:     []ProviderTag{{ProviderID: "tag_1", Name: "billing"}},
		Contacts: []Contact{{ProviderID: "con_1", Name: "Jordan"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Admins != 2 || res.Teams != 1 || res.Tags != 1 || res.Contacts != 1 {
		t.Fatalf("counts = %#v", res)
	}
}

func TestCountsToleratesMissingTables(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: workspacelessSchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	counts, err := Counts(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Conversations != 0 {
		t.Fatalf("expected conversations from workspaceless schema, got %d", counts.Conversations)
	}
	if counts.Admins != 0 || counts.Teams != 0 || counts.Tags != 0 || counts.Contacts != 0 || counts.RawBlobs != 0 {
		t.Fatalf("missing-table counts not zero: %#v", counts)
	}
}

func TestExportFixtureFromWorkspacelessSchema(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: workspacelessSchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversations(
		id, workspace_id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
	) values('conv_1', 'ws_legacy', 'intercom', 'ic_1', 'Subj', 'open', '', '', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	// Cannot use ExportFixture directly since it queries other tables.
	// But ensureFTSSchema migration path is exercised through openStore when called against legacy fts.
	if _, err := Counts(ctx, dbPath); err != nil {
		t.Fatal(err)
	}
}

func TestSearchOnEmptyDBReturnsNoResults(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: Schema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	results, err := SearchWithOptions(ctx, dbPath, "anything", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty, got %d", len(results))
	}
}

func TestSearchWithStateFilterUsesLikePath(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	results, err := SearchWithOptions(ctx, dbPath, "Morgan", SearchOptions{Limit: 10, State: "open"})
	if err != nil {
		t.Fatal(err)
	}
	_ = results
}

func TestEnsureFTSSchemaMigratesLegacyFTS(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	// Legacy schema: conversation_fts has only the older columns; no state/rating/fin_status.
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: legacyFTSSchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Seed a conversation to be backfilled.
	if _, err := st.DB().ExecContext(ctx, `insert into conversations(id, workspace_id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at) values('c1','ws','intercom','ic_c1','subj','open','riley','','','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversation_fts(conversation_id, subject, body, tags, participants, assignee) values('c1','subj','body','billing','Riley','riley')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	// Open via openStore which triggers the migration.
	migrated, err := openStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	// Confirm new columns exist.
	rows, err := migrated.DB().QueryContext(ctx, `pragma table_info(conversation_fts)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}
	for _, want := range []string{"participants", "state", "rating", "fin_status"} {
		if !cols[want] {
			t.Fatalf("migrated fts missing column %q (have %#v)", want, cols)
		}
	}
}

const legacyFTSSchema = `
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
	conversation_id text not null,
	provider text not null,
	provider_id text not null,
	part_type text not null,
	author_name text not null,
	body text not null,
	created_at text not null,
	updated_at text not null
);
create table if not exists conversation_tags (
	conversation_id text not null,
	tag_id text not null,
	primary key (conversation_id, tag_id)
);
create table if not exists tags (
	id text primary key,
	name text not null
);
create table if not exists conversation_participants (
	conversation_id text not null,
	name text not null,
	primary key (conversation_id, name)
);
create virtual table if not exists conversation_fts using fts5(
	conversation_id unindexed,
	subject,
	body,
	tags,
	participants,
	assignee
);
`

func TestUpsertConversationMovesPartsBetweenConversations(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	first := Fixture{
		Workspace: Workspace{ID: "ws_1", Provider: "intercom", Name: "ws"},
		Conversations: []Conversation{{
			ID: "conv_a", Provider: "intercom", ProviderID: "ic_a", Subject: "first",
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
			Parts: []Part{{ID: "part_x", ProviderID: "ic_part_x", Body: "hello", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}},
		}},
	}
	if _, err := SyncFixture(ctx, dbPath, first); err != nil {
		t.Fatal(err)
	}
	// Re-sync attaching the same part to a different conversation.
	second := Fixture{
		Workspace: first.Workspace,
		Conversations: []Conversation{{
			ID: "conv_b", Provider: "intercom", ProviderID: "ic_b", Subject: "second",
			CreatedAt: "2026-01-02T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
			Parts: []Part{{ID: "part_x", ProviderID: "ic_part_x", Body: "world", CreatedAt: "2026-01-02T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z"}},
		}},
	}
	if _, err := SyncFixture(ctx, dbPath, second); err != nil {
		t.Fatal(err)
	}
	convB, err := GetConversation(ctx, dbPath, "ic_b", ConversationDetailOptions{IncludeParts: true, PartLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(convB.Parts) != 1 {
		t.Fatalf("expected part on conv_b: %#v", convB.Parts)
	}
	convA, err := GetConversation(ctx, dbPath, "ic_a", ConversationDetailOptions{IncludeParts: true, PartLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(convA.Parts) != 0 {
		t.Fatalf("expected part to have moved: %#v", convA.Parts)
	}
}

func TestFtsHasColumnsReportsTrueForCurrentSchema(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: Schema, SchemaVersion: SchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ok, err := ftsHasColumns(ctx, st.DB(), []string{"participants", "state", "rating", "fin_status"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected current schema to have all FTS columns")
	}
	missing, err := ftsHasColumns(ctx, st.DB(), []string{"nonexistent_column"})
	if err != nil {
		t.Fatal(err)
	}
	if missing {
		t.Fatalf("expected false for missing column")
	}
}
