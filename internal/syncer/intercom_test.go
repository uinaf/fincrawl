package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
