package store

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveAndLoadSyncState(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	state := SyncState{
		ID:                IntercomTailSyncStateID,
		Provider:          ProviderIntercom,
		CursorKind:        "updated_at",
		HighWaterMark:     "2026-05-16T10:00:00Z",
		ActiveWindowStart: "2026-05-16T09:00:00Z",
		ActiveWindowEnd:   "2026-05-16T11:00:00Z",
		LastProviderID:    "conv_fake_1",
		PageCursor:        "cursor_fake_1",
	}
	if err := SaveSyncState(ctx, dbPath, state); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadSyncState(ctx, dbPath, IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected sync state")
	}
	if loaded.PageCursor != "cursor_fake_1" || loaded.LastProviderID != "conv_fake_1" {
		t.Fatalf("loaded state = %#v", loaded)
	}
	if loaded.UpdatedAt == "" {
		t.Fatalf("updated_at was not set")
	}
}

func TestLoadSyncStateMissing(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	_, ok, err := LoadSyncState(ctx, dbPath, IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("unexpected sync state")
	}
}

func TestListSyncStates(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	for _, state := range []SyncState{
		{ID: "intercom.tail", Provider: ProviderIntercom, CursorKind: "updated_at", HighWaterMark: "2026-05-16T10:00:00Z"},
		{ID: "fake.tail", Provider: "fake", CursorKind: "updated_at", HighWaterMark: "2026-05-16T09:00:00Z"},
	} {
		if err := SaveSyncState(ctx, dbPath, state); err != nil {
			t.Fatal(err)
		}
	}
	states, err := ListSyncStates(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("states = %#v", states)
	}
	if states[0].Provider != "fake" || states[1].Provider != ProviderIntercom {
		t.Fatalf("states were not ordered by provider/id: %#v", states)
	}
}

func TestListSyncStatesDoesNotChangePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permissions are not portable on windows")
	}
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	if err := SaveSyncState(ctx, dbPath, SyncState{
		ID:            IntercomTailSyncStateID,
		Provider:      ProviderIntercom,
		CursorKind:    "updated_at",
		HighWaterMark: "2026-05-16T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dbPath, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dbPath, 0o600)
	})
	states, err := ListSyncStates(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("states = %#v", states)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o400 {
		t.Fatalf("mode = %o, want 400", got)
	}
}

func TestSaveSyncStateUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	first := SyncState{ID: IntercomTailSyncStateID, Provider: ProviderIntercom, CursorKind: "updated_at", HighWaterMark: "2026-05-16T10:00:00Z"}
	if err := SaveSyncState(ctx, dbPath, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.HighWaterMark = "2026-05-17T11:00:00Z"
	second.PageCursor = "page_2"
	if err := SaveSyncState(ctx, dbPath, second); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadSyncState(ctx, dbPath, IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.HighWaterMark != "2026-05-17T11:00:00Z" || loaded.PageCursor != "page_2" {
		t.Fatalf("update lost data: %#v", loaded)
	}
}

func TestSaveSyncStateRequiresFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	if err := SaveSyncState(ctx, dbPath, SyncState{}); err == nil {
		t.Fatalf("empty state should error")
	}
}

func TestGetConversationLooksUpByLocalAndProviderID(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	byProvider, err := GetConversation(ctx, dbPath, "ic_syn_001", ConversationDetailOptions{IncludeParts: true, PartLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if byProvider.ProviderID != "ic_syn_001" {
		t.Fatalf("provider id = %q", byProvider.ProviderID)
	}
	if len(byProvider.Parts) == 0 {
		t.Fatalf("expected parts included")
	}
	byLocal, err := GetConversation(ctx, dbPath, byProvider.ID, ConversationDetailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if byLocal.ProviderID != "ic_syn_001" {
		t.Fatalf("local lookup mismatch: %#v", byLocal)
	}
}

func TestGetConversationReturnsErrorOnUnknownID(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	if _, err := GetConversation(ctx, dbPath, "no_such_id", ConversationDetailOptions{}); err == nil {
		t.Fatalf("expected not-found error")
	}
}
