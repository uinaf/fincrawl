package control

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	ckconfig "github.com/openclaw/crawlkit/config"
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
