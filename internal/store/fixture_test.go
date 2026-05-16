package store

import (
	"context"
	"path/filepath"
	"testing"
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
	if result.RawBlobs != 6 {
		t.Fatalf("raw blobs = %d, want 6", result.RawBlobs)
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
	if _, err := Search(ctx, dbPath, "!!!", 10); err == nil {
		t.Fatalf("empty sanitized search unexpectedly succeeded")
	}
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

func TestSanitizeFTSQuery(t *testing.T) {
	got := SanitizeFTSQuery(`"login" OR billing:refund*`)
	want := "login billing refund"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
