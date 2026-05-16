package store

import (
	"context"
	"path/filepath"
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
