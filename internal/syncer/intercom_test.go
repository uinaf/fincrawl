package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uinaf/fincrawl/internal/intercom"
	"github.com/uinaf/fincrawl/internal/store"
)

func TestNormalizeConversationMapsSearchableFields(t *testing.T) {
	raw := json.RawMessage(`{
		"id": "conv_fake_1",
		"title": "Synthetic billing thread",
		"state": "open",
		"created_at": 1770000000,
		"updated_at": 1770000300,
		"source": {
			"id": "source_fake_1",
			"type": "conversation",
			"body": "Opening message about a fake invoice",
			"author": {"name": "Pat Example"}
		},
		"conversation_parts": {
			"conversation_parts": [{
				"id": "part_fake_1",
				"part_type": "comment",
				"body": "Synthetic refund response",
				"created_at": 1770000100,
				"author": {"name": "Riley Example"}
			}]
		},
		"contacts": {
			"contacts": [{"name": "Pat Example"}]
		},
		"tags": {
			"tags": [{"name": "billing"}, {"name": "refund"}]
		}
	}`)
	conversation, err := NormalizeConversation(intercom.Conversation{ID: "conv_fake_1", Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	if conversation.ProviderID != "conv_fake_1" {
		t.Fatalf("provider id = %q", conversation.ProviderID)
	}
	if conversation.Subject != "Synthetic billing thread" {
		t.Fatalf("subject = %q", conversation.Subject)
	}
	if len(conversation.Parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(conversation.Parts))
	}
	if got := conversation.Tags; len(got) != 2 || got[0] != "billing" || got[1] != "refund" {
		t.Fatalf("tags = %#v", got)
	}
}

func TestSyncConversationHydratesAndIndexesConversation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/conv_fake_1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "conv_fake_1",
			"title": "Synthetic billing thread",
			"state": "open",
			"created_at": 1770000000,
			"updated_at": 1770000300,
			"source": {"id": "source_fake_1", "body": "Opening message", "author": {"name": "Pat Example"}},
			"conversation_parts": {"conversation_parts": [{"id": "part_fake_1", "part_type": "comment", "body": "Synthetic refund response", "created_at": 1770000100, "author": {"name": "Riley Example"}}]},
			"tags": {"tags": [{"name": "billing"}]}
		}`))
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
		Now:    func() time.Time { return time.Unix(1770000000, 0) },
	}
	result, err := s.SyncConversation(context.Background(), dbPath, "conv_fake_1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Conversations != 1 || result.ConversationParts != 2 {
		t.Fatalf("result = %#v", result)
	}
	results, err := store.Search(context.Background(), dbPath, "refund", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderID != "conv_fake_1" {
		t.Fatalf("search results = %#v", results)
	}
}

func TestSyncUpdatedSinceSearchesAndHydratesWithinLimit(t *testing.T) {
	var retrieved int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			w.Write([]byte(`{"conversations":[{"id":"conv_fake_1","updated_at":1770000300},{"id":"conv_fake_2","updated_at":1770000400}],"pages":{"next":{}}}`))
		case "/conversations/conv_fake_1", "/conversations/conv_fake_2":
			retrieved++
			id := r.URL.Path[len("/conversations/"):]
			w.Write([]byte(`{
				"id": "` + id + `",
				"title": "Synthetic search thread",
				"state": "open",
				"created_at": 1770000000,
				"updated_at": 1770000300,
				"conversation_parts": {"conversation_parts": []}
			}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
		Now:    func() time.Time { return time.Unix(1770000000, 0) },
	}
	result, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	if retrieved != 1 {
		t.Fatalf("retrieved = %d, want 1", retrieved)
	}
	if result.Conversations != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSyncUpdatedSinceReportsWorkspaceForEmptyWindows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"conversations":[],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
		Now:    func() time.Time { return time.Unix(1770000000, 0) },
	}
	result, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkspaceID != "intercom" {
		t.Fatalf("workspace id = %q, want intercom", result.WorkspaceID)
	}
}

func TestResumeTailContinuesAfterLimitedRun(t *testing.T) {
	var retrieved []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			w.Write([]byte(`{"conversations":[{"id":"conv_fake_1","updated_at":1770000300},{"id":"conv_fake_2","updated_at":1770000400}],"pages":{"next":{}}}`))
		case "/conversations/conv_fake_1", "/conversations/conv_fake_2":
			id := r.URL.Path[len("/conversations/"):]
			retrieved = append(retrieved, id)
			w.Write([]byte(`{
				"id": "` + id + `",
				"title": "Synthetic resume thread",
				"state": "open",
				"created_at": 1770000000,
				"updated_at": 1770000300,
				"conversation_parts": {"conversation_parts": []}
			}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
		Now:    func() time.Time { return time.Unix(1770000000, 0) },
	}
	first, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Conversations != 1 {
		t.Fatalf("first result = %#v", first)
	}
	state, ok, err := store.LoadSyncState(context.Background(), dbPath, store.IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state.LastProviderID != "conv_fake_1" || state.ActiveWindowEnd == "" {
		t.Fatalf("state after first run = %#v", state)
	}
	second, err := s.ResumeTail(context.Background(), dbPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if second.Conversations != 1 {
		t.Fatalf("second result = %#v", second)
	}
	if len(retrieved) != 2 || retrieved[0] != "conv_fake_1" || retrieved[1] != "conv_fake_2" {
		t.Fatalf("retrieved = %#v", retrieved)
	}
	state, ok, err = store.LoadSyncState(context.Background(), dbPath, store.IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state.ActiveWindowEnd != "" || state.LastProviderID != "" || state.HighWaterMark == "" {
		t.Fatalf("state after resume = %#v", state)
	}
	counts, err := store.Counts(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Conversations != 2 {
		t.Fatalf("conversation count = %d, want 2", counts.Conversations)
	}
}

func TestSyncUpdatedSinceRequiresResumeWhenActive(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	if err := store.SaveSyncState(context.Background(), dbPath, store.SyncState{
		ID:                store.IntercomTailSyncStateID,
		ActiveWindowStart: "2026-05-16T10:00:00Z",
		ActiveWindowEnd:   "2026-05-16T11:00:00Z",
		LastProviderID:    "conv_fake_1",
	}); err != nil {
		t.Fatal(err)
	}
	s := IntercomSyncer{}
	_, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 1)
	if err == nil || !strings.Contains(err.Error(), "sync --resume") {
		t.Fatalf("expected resume-first error, got %v", err)
	}
	state, ok, err := store.LoadSyncState(context.Background(), dbPath, store.IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state.ActiveWindowEnd == "" || state.LastProviderID != "conv_fake_1" {
		t.Fatalf("state after rejected fresh window = %#v", state)
	}
}

func TestResumeTailRequiresActiveState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{}
	if _, err := s.ResumeTail(context.Background(), dbPath, 1); err == nil {
		t.Fatalf("expected missing active state error")
	}
}

func TestResumeTailRejectsCorruptWindow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	if err := store.SaveSyncState(context.Background(), dbPath, store.SyncState{
		ID:                store.IntercomTailSyncStateID,
		ActiveWindowStart: "not-a-time",
		ActiveWindowEnd:   "2026-05-16T11:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	s := IntercomSyncer{}
	if _, err := s.ResumeTail(context.Background(), dbPath, 1); err == nil {
		t.Fatalf("expected corrupt active window error")
	}
}

func TestResumeTailStopsWhenMarkerIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			w.Write([]byte(`{"conversations":[{"id":"conv_fake_2","updated_at":1770000400}],"pages":{"next":{}}}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	if err := store.SaveSyncState(context.Background(), dbPath, store.SyncState{
		ID:                store.IntercomTailSyncStateID,
		ActiveWindowStart: "2026-05-16T10:00:00Z",
		ActiveWindowEnd:   "2026-05-16T11:00:00Z",
		LastProviderID:    "conv_missing",
	}); err != nil {
		t.Fatal(err)
	}
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
	}
	_, err := s.ResumeTail(context.Background(), dbPath, 0)
	if err == nil || !strings.Contains(err.Error(), "resume marker") {
		t.Fatalf("expected missing resume marker error, got %v", err)
	}
	state, ok, err := store.LoadSyncState(context.Background(), dbPath, store.IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state.ActiveWindowEnd == "" || state.LastProviderID != "conv_missing" || state.HighWaterMark != "" {
		t.Fatalf("state after missing marker = %#v", state)
	}
	counts, err := store.Counts(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Conversations != 0 {
		t.Fatalf("conversation count = %d, want 0", counts.Conversations)
	}
}

func TestSyncUpdatedSinceRetriesRateLimitedSearch(t *testing.T) {
	var searches int
	var slept time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			searches++
			if searches == 1 {
				w.Header().Set("Retry-After", "2")
				http.Error(w, "slow down", http.StatusTooManyRequests)
				return
			}
			w.Write([]byte(`{"conversations":[{"id":"conv_fake_1","updated_at":1770000300}],"pages":{"next":{}}}`))
		case "/conversations/conv_fake_1":
			w.Write([]byte(`{
				"id": "conv_fake_1",
				"title": "Synthetic retry thread",
				"state": "open",
				"created_at": 1770000000,
				"updated_at": 1770000300,
				"conversation_parts": {"conversation_parts": []}
			}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{
			BaseURL:    server.URL,
			Token:      "fake-token",
			HTTPClient: server.Client(),
			Sleep: func(ctx context.Context, d time.Duration) error {
				slept = d
				return nil
			},
		},
	}
	result, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	if searches != 2 {
		t.Fatalf("searches = %d, want 2", searches)
	}
	if slept != 2*time.Second {
		t.Fatalf("slept = %s, want 2s", slept)
	}
	if result.Conversations != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSyncConversationRetriesRateLimitedHydration(t *testing.T) {
	var retrieves int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/conv_fake_1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		retrieves++
		w.Header().Set("Content-Type", "application/json")
		if retrieves == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{
			"id": "conv_fake_1",
			"title": "Synthetic retry thread",
			"state": "open",
			"created_at": 1770000000,
			"updated_at": 1770000300,
			"conversation_parts": {"conversation_parts": []}
		}`))
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{
			BaseURL:    server.URL,
			Token:      "fake-token",
			HTTPClient: server.Client(),
			Sleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
		},
	}
	result, err := s.SyncConversation(context.Background(), dbPath, "conv_fake_1")
	if err != nil {
		t.Fatal(err)
	}
	if retrieves != 2 {
		t.Fatalf("retrieves = %d, want 2", retrieves)
	}
	if result.Conversations != 1 {
		t.Fatalf("result = %#v", result)
	}
}
