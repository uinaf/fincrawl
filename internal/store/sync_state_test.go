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
