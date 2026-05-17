package store

import (
	"context"
	"path/filepath"
	"testing"

	ckstore "github.com/openclaw/crawlkit/store"
)

func TestExportFixtureRoundTrip(t *testing.T) {
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "archive.sqlite")
	if _, err := SyncFixture(context.Background(), dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	got, err := ExportFixture(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace.ID != fixture.Workspace.ID {
		t.Fatalf("workspace ID = %q, want %q", got.Workspace.ID, fixture.Workspace.ID)
	}
	if len(got.Entities.Tags) != len(fixture.Entities.Tags) {
		t.Fatalf("tags = %d, want %d", len(got.Entities.Tags), len(fixture.Entities.Tags))
	}
	if len(got.Conversations) != len(fixture.Conversations) {
		t.Fatalf("conversations = %d, want %d", len(got.Conversations), len(fixture.Conversations))
	}
	if len(got.Conversations[0].Parts) == 0 {
		t.Fatalf("missing conversation parts")
	}
	if len(got.Conversations[0].Raw) == 0 {
		t.Fatalf("missing raw conversation blob")
	}
}

func TestExportFixtureToleratesOptionalLegacyTablesMissing(t *testing.T) {
	ctx := context.Background()
	fixture, err := LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "archive.sqlite")
	if _, err := SyncFixture(ctx, dbPath, fixture); err != nil {
		t.Fatal(err)
	}
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`drop table conversation_tags`,
		`drop table tags`,
		`drop table conversation_participants`,
		`drop table provider_tags`,
		`drop table contacts`,
		`drop table admins`,
		`drop table teams`,
		`drop table raw_blobs`,
	} {
		if _, err := st.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ExportFixture(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace.ID != fixture.Workspace.ID {
		t.Fatalf("workspace ID = %q, want %q", got.Workspace.ID, fixture.Workspace.ID)
	}
	if len(got.Entities.Admins) != 0 || len(got.Entities.Tags) != 0 || len(got.Entities.Contacts) != 0 {
		t.Fatalf("entities = %#v", got.Entities)
	}
	if len(got.Conversations) != len(fixture.Conversations) {
		t.Fatalf("conversations = %d, want %d", len(got.Conversations), len(fixture.Conversations))
	}
	if len(got.Conversations[0].Parts) == 0 {
		t.Fatalf("missing conversation parts")
	}
}
