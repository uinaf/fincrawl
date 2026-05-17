package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	ckstore "github.com/openclaw/crawlkit/store"
)

func TestSyncFixtureAndSearch(t *testing.T) {
	ctx := context.Background()
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	result, err := SyncFixture(ctx, dbPath, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if result.Conversations != 2 {
		t.Fatalf("conversations = %d, want 2", result.Conversations)
	}
	if result.ConversationParts != 4 {
		t.Fatalf("parts = %d, want 4", result.ConversationParts)
	}
	if result.Admins != 2 || result.Teams != 1 || result.Tags != 2 || result.Contacts != 2 {
		t.Fatalf("entity counts = admins %d teams %d tags %d contacts %d", result.Admins, result.Teams, result.Tags, result.Contacts)
	}
	if result.RawBlobs != 13 {
		t.Fatalf("raw blobs = %d, want 13", result.RawBlobs)
	}
	second, err := SyncFixture(ctx, dbPath, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if second.RawBlobs != 0 {
		t.Fatalf("second raw blobs = %d, want 0 inserted blobs", second.RawBlobs)
	}
	results, err := Search(ctx, dbPath, "login code", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].ProviderID != "ic_syn_002" {
		t.Fatalf("provider id = %s, want ic_syn_002", results[0].ProviderID)
	}
	if results[0].Rating != "neutral" || results[0].FinStatus != "resolved" {
		t.Fatalf("metadata = rating %q fin_status %q", results[0].Rating, results[0].FinStatus)
	}
	if !contains(results[0].Participants, "Jordan Customer") {
		t.Fatalf("participants = %#v, want Jordan Customer", results[0].Participants)
	}
	results, err = Search(ctx, dbPath, "Morgan", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("assignee results = %d, want 1", len(results))
	}
	if results[0].ProviderID != "ic_syn_002" {
		t.Fatalf("assignee provider id = %s, want ic_syn_002", results[0].ProviderID)
	}
	results, err = Search(ctx, dbPath, "Jordan", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_syn_002" {
		t.Fatalf("participant results = %#v", results)
	}
	results, err = Search(ctx, dbPath, "resolved", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_syn_002" {
		t.Fatalf("fin status results = %#v", results)
	}
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var admins int
	if err := st.DB().QueryRowContext(ctx, `select count(*) from admins`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 2 {
		t.Fatalf("admins in store = %d, want 2", admins)
	}
	if _, err := Search(ctx, dbPath, "!!!", 10); err == nil {
		t.Fatalf("empty sanitized search unexpectedly succeeded")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSyncConversationsDefaultsWorkspaceForEmptyImports(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")

	result, err := SyncConversations(ctx, dbPath, Workspace{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkspaceID != ProviderIntercom {
		t.Fatalf("workspace id = %q, want %q", result.WorkspaceID, ProviderIntercom)
	}
}

func TestSyncFixtureMigratesLegacyFTSShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	legacySchema := strings.Replace(v1Schema, "participants,\n\tassignee", "assignee", 1)
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: legacySchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	results, err := Search(ctx, dbPath, "Jordan", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_syn_002" {
		t.Fatalf("participant search after migration = %#v", results)
	}
}

func TestSearchReadsLegacyArchiveWithoutParticipantTable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	createLegacySearchArchive(t, ctx, dbPath)
	results, err := Search(ctx, dbPath, "Legacy Participant", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_legacy_001" {
		t.Fatalf("legacy participant search = %#v", results)
	}
	if !contains(results[0].Participants, "Legacy Participant") {
		t.Fatalf("participants = %#v", results[0].Participants)
	}
	results, err = Search(ctx, dbPath, "resolved", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_legacy_001" {
		t.Fatalf("legacy metadata search = %#v", results)
	}
}

func TestSyncPreservesLegacyFTSParticipants(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	createLegacySearchArchive(t, ctx, dbPath)
	if _, err := SyncEntities(ctx, dbPath, Workspace{}, Entities{}); err != nil {
		t.Fatal(err)
	}
	results, err := Search(ctx, dbPath, "Legacy Participant", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_legacy_001" {
		t.Fatalf("legacy participant search after migration = %#v", results)
	}
	if !contains(results[0].Participants, "Legacy Participant") {
		t.Fatalf("participants after migration = %#v", results[0].Participants)
	}
}

func createLegacySearchArchive(t *testing.T, ctx context.Context, dbPath string) {
	t.Helper()
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: v1Schema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB().ExecContext(ctx, `insert into workspaces(id, provider, name, created_at) values('workspace_legacy', 'intercom', 'Legacy Workspace', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversations(
		id, workspace_id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
	) values('conv_legacy_001', 'workspace_legacy', 'intercom', 'ic_legacy_001', 'Legacy subject', 'open', '', 'neutral', 'resolved', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversation_fts(conversation_id, subject, body, tags, participants, assignee)
		values('conv_legacy_001', 'Legacy subject', 'Legacy body', '', 'Legacy Participant', '')`); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeFTSQuery(t *testing.T) {
	got := SanitizeFTSQuery(`"login" OR billing:refund*`)
	want := "login billing refund"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

const v1Schema = `
create table if not exists workspaces (
	id text primary key,
	provider text not null,
	name text not null,
	created_at text not null
);

create table if not exists conversations (
	id text primary key,
	workspace_id text not null references workspaces(id),
	provider text not null,
	provider_id text not null,
	subject text not null,
	state text not null,
	assignee text not null default '',
	rating text not null default '',
	fin_status text not null default '',
	created_at text not null,
	updated_at text not null,
	unique(provider, provider_id)
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
	updated_at text not null,
	unique(provider, provider_id)
);

create table if not exists tags (
	id text primary key,
	name text not null unique
);

create table if not exists conversation_tags (
	conversation_id text not null references conversations(id) on delete cascade,
	tag_id text not null references tags(id) on delete cascade,
	primary key(conversation_id, tag_id)
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

create virtual table if not exists conversation_fts using fts5(
	conversation_id unindexed,
	subject,
	body,
	tags,
	participants,
	assignee
);
`
