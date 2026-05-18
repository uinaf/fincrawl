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
	results, err = SearchWithOptions(ctx, dbPath, "invoice", SearchOptions{Limit: 10, State: "open", Tag: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_syn_001" {
		t.Fatalf("filtered invoice results = %#v", results)
	}
	results, err = SearchWithOptions(ctx, dbPath, "invoice", SearchOptions{Limit: 10, State: "closed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("closed invoice results = %#v, want none", results)
	}
	results, err = SearchWithOptions(ctx, dbPath, "login", SearchOptions{Limit: 10, FinStatus: "resolved"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "ic_syn_002" {
		t.Fatalf("fin-filtered login results = %#v", results)
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

func TestGetConversationShowsSanitizedDetails(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	workspace := Workspace{ID: "synthetic_workspace", Provider: ProviderIntercom, Name: "Synthetic Workspace", CreatedAt: "2026-01-01T00:00:00Z"}
	conversation := Conversation{
		ID:           "conversation_syn_sanitized",
		Provider:     ProviderIntercom,
		ProviderID:   "syn_sanitized",
		Subject:      "Synthetic sanitized thread",
		State:        "open",
		Participants: []string{"Control Example"},
		Tags:         []string{"synthetic"},
		CreatedAt:    "2026-01-01T00:00:00Z",
		UpdatedAt:    "2026-01-01T01:00:00Z",
		Parts: []Part{
			{ID: "part_syn_sanitized", ProviderID: "part_syn_sanitized", Type: "comment", AuthorName: "Synthetic User", Body: "hello\n\tworld with   space", CreatedAt: "2026-01-01T00:05:00Z", UpdatedAt: "2026-01-01T00:05:00Z"},
		},
	}
	if _, err := SyncConversations(ctx, dbPath, workspace, []Conversation{conversation}); err != nil {
		t.Fatal(err)
	}

	detail, err := GetConversation(ctx, dbPath, "syn_sanitized", ConversationDetailOptions{IncludeParts: true, PartLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != "conversation_syn_sanitized" || detail.ProviderID != "syn_sanitized" {
		t.Fatalf("detail = %#v", detail)
	}
	if detail.Snippet != "hello world with space" {
		t.Fatalf("snippet = %q", detail.Snippet)
	}
	if len(detail.Parts) != 1 || detail.Parts[0].Body != "hello world with space" {
		t.Fatalf("parts = %#v", detail.Parts)
	}
}

func TestGetConversationReadsLegacyArchiveWithoutOptionalTables(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	const legacyDetailSchema = `
create table if not exists conversations (
	id text primary key,
	provider_id text not null,
	subject text not null,
	state text not null,
	assignee text not null default '',
	rating text not null default '',
	fin_status text not null default '',
	created_at text not null,
	updated_at text not null
);
create table if not exists conversation_parts (
	id text primary key,
	conversation_id text not null,
	provider_id text not null,
	part_type text not null,
	author_name text not null,
	body text not null,
	created_at text not null,
	updated_at text not null
);
`
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: legacyDetailSchema, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversations(
		id, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
	) values('legacy_detail', 'ic_legacy_detail', 'Legacy detail', 'open', '', '', '', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `insert into conversation_parts(
		id, conversation_id, provider_id, part_type, author_name, body, created_at, updated_at
	) values('legacy_part', 'legacy_detail', 'legacy_part', 'comment', 'Synthetic User', 'legacy body', '2026-01-01T01:00:00Z', '2026-01-01T01:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	detail, err := GetConversation(ctx, dbPath, "ic_legacy_detail", ConversationDetailOptions{IncludeParts: true, PartLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if detail.ProviderID != "ic_legacy_detail" || detail.Snippet != "legacy body" {
		t.Fatalf("detail = %#v", detail)
	}
	if len(detail.Parts) != 1 || detail.Parts[0].Body != "legacy body" {
		t.Fatalf("parts = %#v", detail.Parts)
	}
	if len(detail.Tags) != 0 || len(detail.Participants) != 0 {
		t.Fatalf("optional data leaked from missing tables: %#v", detail)
	}
}

func TestSearchReturnsScoresAndSanitizedSnippets(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	workspace := Workspace{ID: "synthetic_workspace", Provider: ProviderIntercom, Name: "Synthetic Workspace", CreatedAt: "2026-01-01T00:00:00Z"}
	conversations := []Conversation{
		{
			ID:         "conversation_syn_alpha_subject",
			Provider:   ProviderIntercom,
			ProviderID: "syn_alpha_subject",
			Subject:    "Alpha exact topic",
			State:      "open",
			CreatedAt:  "2026-01-01T00:00:00Z",
			UpdatedAt:  "2026-01-01T01:00:00Z",
			Parts:      []Part{{ID: "part_syn_alpha_1", ProviderID: "part_syn_alpha_1", Type: "comment", Body: "boring body", CreatedAt: "2026-01-01T00:05:00Z", UpdatedAt: "2026-01-01T00:05:00Z"}},
		},
		{
			ID:         "conversation_syn_alpha_body",
			Provider:   ProviderIntercom,
			ProviderID: "syn_alpha_body",
			Subject:    "Different subject",
			State:      "open",
			CreatedAt:  "2026-01-02T00:00:00Z",
			UpdatedAt:  "2026-01-02T01:00:00Z",
			Parts:      []Part{{ID: "part_syn_alpha_2", ProviderID: "part_syn_alpha_2", Type: "comment", Body: "alpha in\nbody", CreatedAt: "2026-01-02T00:05:00Z", UpdatedAt: "2026-01-02T00:05:00Z"}},
		},
	}
	if _, err := SyncConversations(ctx, dbPath, workspace, conversations); err != nil {
		t.Fatal(err)
	}
	results, err := Search(ctx, dbPath, "alpha", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].ProviderID != "syn_alpha_subject" {
		t.Fatalf("top result = %#v, want subject hit before newer body-only hit", results)
	}
	if results[0].Score == 0 || results[1].Score == 0 || results[0].Score < results[1].Score {
		t.Fatalf("scores were not ordered: %#v", results)
	}
	for _, result := range results {
		if strings.ContainsAny(result.Snippet, "\x00\n\t") {
			t.Fatalf("unsafe snippet = %q", result.Snippet)
		}
	}
}

func TestSearchLikeFallbackOrdersByScore(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	workspace := Workspace{ID: "synthetic_workspace", Provider: ProviderIntercom, Name: "Synthetic Workspace", CreatedAt: "2026-01-01T00:00:00Z"}
	conversations := []Conversation{
		{
			ID:         "fallback_subject_id",
			Provider:   ProviderIntercom,
			ProviderID: "fallback_subject",
			Subject:    "Alpha exact topic",
			State:      "open",
			CreatedAt:  "2026-01-01T00:00:00Z",
			UpdatedAt:  "2026-01-01T01:00:00Z",
			Parts:      []Part{{ID: "fallback_part_subject", ProviderID: "fallback_part_subject", Type: "comment", Body: "boring body", CreatedAt: "2026-01-01T00:05:00Z", UpdatedAt: "2026-01-01T00:05:00Z"}},
		},
		{
			ID:         "fallback_body_id",
			Provider:   ProviderIntercom,
			ProviderID: "fallback_body",
			Subject:    "Different subject",
			State:      "open",
			CreatedAt:  "2026-01-02T00:00:00Z",
			UpdatedAt:  "2026-01-02T01:00:00Z",
			Parts:      []Part{{ID: "fallback_part_body", ProviderID: "fallback_part_body", Type: "comment", Body: "alpha in body", CreatedAt: "2026-01-02T00:05:00Z", UpdatedAt: "2026-01-02T00:05:00Z"}},
		},
	}
	if _, err := SyncConversations(ctx, dbPath, workspace, conversations); err != nil {
		t.Fatal(err)
	}
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: Schema, SchemaVersion: SchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `drop table conversation_fts`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	results, err := Search(ctx, dbPath, "alpha", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].ProviderID != "fallback_subject" {
		t.Fatalf("top result = %#v, want subject hit before newer body-only hit", results)
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("scores were not ordered: %#v", results)
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

func TestSyncConversationsUpsertsDuplicateProviderParts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	workspace := Workspace{ID: "synthetic_workspace", Provider: ProviderIntercom, Name: "Synthetic Workspace", CreatedAt: "2026-01-01T00:00:00Z"}
	conversations := []Conversation{
		{
			ID:         "conversation_syn_001",
			Provider:   ProviderIntercom,
			ProviderID: "syn_001",
			Subject:    "Synthetic first thread",
			State:      "open",
			CreatedAt:  "2026-01-01T00:00:00Z",
			UpdatedAt:  "2026-01-01T01:00:00Z",
			Parts: []Part{
				{ID: "part_syn_shared", ProviderID: "syn_shared", Type: "comment", AuthorName: "Synthetic User", Body: "Original synthetic body", CreatedAt: "2026-01-01T00:05:00Z", UpdatedAt: "2026-01-01T00:05:00Z"},
			},
		},
		{
			ID:         "conversation_syn_002",
			Provider:   ProviderIntercom,
			ProviderID: "syn_002",
			Subject:    "Synthetic second thread",
			State:      "open",
			CreatedAt:  "2026-01-01T02:00:00Z",
			UpdatedAt:  "2026-01-01T03:00:00Z",
			Parts: []Part{
				{ID: "part_syn_shared", ProviderID: "syn_shared", Type: "comment", AuthorName: "Synthetic User", Body: "Duplicate synthetic body", CreatedAt: "2026-01-01T02:05:00Z", UpdatedAt: "2026-01-01T02:05:00Z"},
				{ID: "part_syn_shared", ProviderID: "syn_shared", Type: "comment", AuthorName: "Synthetic User", Body: "Latest duplicate synthetic body", CreatedAt: "2026-01-01T02:06:00Z", UpdatedAt: "2026-01-01T02:06:00Z"},
			},
		},
	}

	if _, err := SyncConversations(ctx, dbPath, workspace, conversations); err != nil {
		t.Fatal(err)
	}

	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var count int
	var conversationID string
	var body string
	if err := st.DB().QueryRowContext(ctx, `select count(*), max(conversation_id), max(body) from conversation_parts where provider = ? and provider_id = ?`, ProviderIntercom, "syn_shared").Scan(&count, &conversationID, &body); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate part rows = %d, want 1", count)
	}
	if conversationID != "conversation_syn_002" {
		t.Fatalf("duplicate part conversation = %q, want conversation_syn_002", conversationID)
	}
	if body != "Latest duplicate synthetic body" {
		t.Fatalf("duplicate part body = %q, want latest body", body)
	}
	results, err := Search(ctx, dbPath, "Original synthetic body", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("stale duplicate body results = %#v, want none", results)
	}
	results, err = Search(ctx, dbPath, "Latest duplicate synthetic body", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "syn_002" {
		t.Fatalf("latest duplicate body results = %#v", results)
	}
}

func TestSyncConversationsNormalizesBlankPartProviderIDs(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	workspace := Workspace{ID: "synthetic_workspace", Provider: ProviderIntercom, Name: "Synthetic Workspace", CreatedAt: "2026-01-01T00:00:00Z"}
	conversations := []Conversation{
		{
			ID:         "conversation_syn_blank_parts",
			Provider:   ProviderIntercom,
			ProviderID: "syn_blank_parts",
			Subject:    "Synthetic blank provider parts",
			State:      "open",
			CreatedAt:  "2026-01-01T00:00:00Z",
			UpdatedAt:  "2026-01-01T01:00:00Z",
			Parts: []Part{
				{ID: "part_syn_blank_001", Type: "comment", AuthorName: "Synthetic User", Body: "Synthetic blank one", CreatedAt: "2026-01-01T00:05:00Z", UpdatedAt: "2026-01-01T00:05:00Z"},
				{ID: "part_syn_blank_002", Type: "comment", AuthorName: "Synthetic User", Body: "Synthetic blank two", CreatedAt: "2026-01-01T00:06:00Z", UpdatedAt: "2026-01-01T00:06:00Z"},
			},
		},
	}

	if _, err := SyncConversations(ctx, dbPath, workspace, conversations); err != nil {
		t.Fatal(err)
	}

	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var count int
	if err := st.DB().QueryRowContext(ctx, `select count(*) from conversation_parts where conversation_id = ?`, "conversation_syn_blank_parts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("blank provider part rows = %d, want 2", count)
	}
	results, err := Search(ctx, dbPath, "Synthetic blank one", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "syn_blank_parts" {
		t.Fatalf("blank provider part search results = %#v", results)
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
