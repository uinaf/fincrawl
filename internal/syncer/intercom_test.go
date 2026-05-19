package syncer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ckstore "github.com/openclaw/crawlkit/store"
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

func TestSyncEntitiesHydratesReadOnlyWorkspaceMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admins":
			w.Write([]byte(`{"admins":[{"id":"admin_syn_1","name":"Riley Example","email":"riley@example.invalid","team_ids":["team_syn_1"]}]}`))
		case "/teams":
			w.Write([]byte(`{"teams":[{"id":"team_syn_1","name":"Synthetic Support"}]}`))
		case "/tags":
			w.Write([]byte(`{"tags":[{"id":"tag_syn_1","name":"billing"}]}`))
		case "/contacts":
			if r.URL.Query().Get("per_page") != "2" {
				t.Fatalf("per_page = %q", r.URL.Query().Get("per_page"))
			}
			w.Write([]byte(`{"data":[{"id":"contact_syn_1","name":"Casey Example"},{"id":"contact_syn_2","name":"Jordan Example"}],"pages":{"next":{}}}`))
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
	result, err := s.SyncEntities(context.Background(), dbPath, EntitySyncOptions{IncludeContacts: true, ContactLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Admins != 1 || result.Teams != 1 || result.Tags != 1 || result.Contacts != 2 {
		t.Fatalf("result = %#v", result)
	}
	st, err := ckstore.OpenReadOnly(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var contacts int
	if err := st.DB().QueryRowContext(context.Background(), `select count(*) from contacts`).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 2 {
		t.Fatalf("contacts = %d, want 2", contacts)
	}
}

func TestSyncEntitiesTreatsDeniedOptionalScopesAsWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admins":
			http.Error(w, "missing scope", http.StatusForbidden)
		case "/teams":
			w.Write([]byte(`{"teams":[{"id":"team_syn_1","name":"Synthetic Support"}]}`))
		case "/tags":
			w.Write([]byte(`{"tags":[{"id":"tag_syn_1","name":"billing"}]}`))
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
	result, err := s.SyncEntities(context.Background(), dbPath, EntitySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Admins != 0 || result.Teams != 1 || result.Tags != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "admins unavailable") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestSyncEntitiesTreatsUnauthorizedOptionalScopeAsWarningWhenOthersWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admins":
			http.Error(w, "missing scope", http.StatusUnauthorized)
		case "/teams":
			w.Write([]byte(`{"teams":[{"id":"team_syn_1","name":"Synthetic Support"}]}`))
		case "/tags":
			w.Write([]byte(`{"tags":[{"id":"tag_syn_1","name":"billing"}]}`))
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
	result, err := s.SyncEntities(context.Background(), dbPath, EntitySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Teams != 1 || result.Tags != 1 || len(result.Warnings) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSyncEntitiesAllowsSuccessfulEmptyScopesWithWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admins":
			http.Error(w, "missing scope", http.StatusForbidden)
		case "/teams":
			w.Write([]byte(`{"teams":[]}`))
		case "/tags":
			w.Write([]byte(`{"tags":[]}`))
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
	result, err := s.SyncEntities(context.Background(), dbPath, EntitySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Admins != 0 || result.Teams != 0 || result.Tags != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "admins unavailable") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestSyncEntitiesFailsInvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
	}
	_, err := s.SyncEntities(context.Background(), dbPath, EntitySyncOptions{})
	if err == nil || !strings.Contains(err.Error(), "no Intercom entity scopes available") {
		t.Fatalf("expected no usable entity scopes error, got %v", err)
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

func TestSyncUpdatedSinceFollowsSearchPagination(t *testing.T) {
	var searches []string
	var retrieved []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			var body struct {
				Pagination map[string]any `json:"pagination"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			cursor, _ := body.Pagination["starting_after"].(string)
			searches = append(searches, cursor)
			if cursor == "" {
				w.Write([]byte(`{"conversations":[{"id":"conv_fake_1","updated_at":1770000300}],"pages":{"next":{"starting_after":"cursor_2"}}}`))
				return
			}
			if cursor != "cursor_2" {
				t.Fatalf("cursor = %q, want cursor_2", cursor)
			}
			w.Write([]byte(`{"conversations":[{"id":"conv_fake_2","updated_at":1770000400}],"pages":{"next":{}}}`))
		case "/conversations/conv_fake_1", "/conversations/conv_fake_2":
			id := r.URL.Path[len("/conversations/"):]
			retrieved = append(retrieved, id)
			w.Write([]byte(`{
				"id": "` + id + `",
				"title": "Synthetic paginated thread",
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
	result, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(searches) != 2 || searches[0] != "" || searches[1] != "cursor_2" {
		t.Fatalf("search cursors = %#v", searches)
	}
	if len(retrieved) != 2 || retrieved[0] != "conv_fake_1" || retrieved[1] != "conv_fake_2" {
		t.Fatalf("retrieved = %#v", retrieved)
	}
	if result.Conversations != 2 {
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

func TestSyncUpdatedSinceIncludesAdjacentWindowBoundarySeconds(t *testing.T) {
	var searchBounds [][]map[string]any
	var retrieved []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			var body struct {
				Query struct {
					Value []map[string]any `json:"value"`
				} `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			searchBounds = append(searchBounds, body.Query.Value)
			w.Write([]byte(`{"conversations":[{"id":"conv_boundary","updated_at":200}],"pages":{"next":{}}}`))
		case "/conversations/conv_boundary":
			retrieved = append(retrieved, "conv_boundary")
			w.Write([]byte(`{
				"id": "conv_boundary",
				"title": "Synthetic boundary thread",
				"state": "open",
				"created_at": 100,
				"updated_at": 200,
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

	if _, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(100, 0), time.Unix(200, 0), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(200, 0), time.Unix(300, 0), 0); err != nil {
		t.Fatal(err)
	}
	if len(searchBounds) != 2 {
		t.Fatalf("searches = %#v", searchBounds)
	}
	if searchBounds[0][0]["value"] != float64(99) || searchBounds[0][1]["value"] != float64(201) {
		t.Fatalf("first window bounds = %#v", searchBounds[0])
	}
	if searchBounds[1][0]["value"] != float64(199) || searchBounds[1][1]["value"] != float64(301) {
		t.Fatalf("second window bounds = %#v", searchBounds[1])
	}
	if len(retrieved) != 2 {
		t.Fatalf("retrieved = %#v, want boundary conversation covered by both adjacent windows", retrieved)
	}
	counts, err := store.Counts(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Conversations != 1 {
		t.Fatalf("conversation count = %d, want idempotent boundary upsert", counts.Conversations)
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

func TestResumeTailContinuesAcrossPagesAfterLimitedRun(t *testing.T) {
	var searches []string
	var retrieved []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/conversations/search":
			var body struct {
				Pagination map[string]any `json:"pagination"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			cursor, _ := body.Pagination["starting_after"].(string)
			searches = append(searches, cursor)
			if cursor == "" {
				w.Write([]byte(`{"conversations":[{"id":"conv_fake_1","updated_at":1770000300}],"pages":{"next":{"starting_after":"cursor_2"}}}`))
				return
			}
			if cursor != "cursor_2" {
				t.Fatalf("cursor = %q, want cursor_2", cursor)
			}
			w.Write([]byte(`{"conversations":[{"id":"conv_fake_2","updated_at":1770000400}],"pages":{"next":{}}}`))
		case "/conversations/conv_fake_1", "/conversations/conv_fake_2":
			id := r.URL.Path[len("/conversations/"):]
			retrieved = append(retrieved, id)
			w.Write([]byte(`{
				"id": "` + id + `",
				"title": "Synthetic paged resume thread",
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
	second, err := s.ResumeTail(context.Background(), dbPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if second.Conversations != 1 {
		t.Fatalf("second result = %#v", second)
	}
	if len(searches) != 3 || searches[0] != "" || searches[1] != "" || searches[2] != "cursor_2" {
		t.Fatalf("search cursors = %#v", searches)
	}
	if len(retrieved) != 2 || retrieved[0] != "conv_fake_1" || retrieved[1] != "conv_fake_2" {
		t.Fatalf("retrieved = %#v", retrieved)
	}
	counts, err := store.Counts(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Conversations != 2 {
		t.Fatalf("conversation count = %d, want 2", counts.Conversations)
	}
}

func TestSyncUpdatedSincePreservesNewerHighWaterMark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"conversations":[],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "fincrawl.db")
	newer := time.Unix(1770001000, 0).UTC().Format(time.RFC3339)
	if err := store.SaveSyncState(context.Background(), dbPath, store.SyncState{
		ID:            store.IntercomTailSyncStateID,
		HighWaterMark: newer,
	}); err != nil {
		t.Fatal(err)
	}
	s := IntercomSyncer{
		Client: intercom.Client{BaseURL: server.URL, Token: "fake-token", HTTPClient: server.Client()},
		Now:    func() time.Time { return time.Unix(1770000000, 0) },
	}
	_, err := s.SyncUpdatedSince(context.Background(), dbPath, time.Unix(1769990000, 0), time.Unix(1770000500, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	state, ok, err := store.LoadSyncState(context.Background(), dbPath, store.IntercomTailSyncStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state.HighWaterMark != newer {
		t.Fatalf("state = %#v, want high water mark %q", state, newer)
	}
}

func TestAdvanceHighWaterMarkPreservesValidCurrentFromMalformedCandidate(t *testing.T) {
	current := "2026-05-17T18:00:00Z"
	if got := advanceHighWaterMark(current, "not-a-timestamp"); got != current {
		t.Fatalf("high water mark = %q, want %q", got, current)
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

func TestFirstWordsTrimsAndCaps(t *testing.T) {
	cases := []struct {
		in    string
		count int
		want  string
	}{
		{"  hello   there  friend  ", 2, "hello there"},
		{"one two three", 5, "one two three"},
		{"", 3, ""},
		{"   ", 1, ""},
		{"single", 1, "single"},
	}
	for _, tc := range cases {
		if got := firstWords(tc.in, tc.count); got != tc.want {
			t.Fatalf("firstWords(%q, %d) = %q, want %q", tc.in, tc.count, got, tc.want)
		}
	}
}

func TestNormalizeAssigneeFallsBackToIDs(t *testing.T) {
	raw := map[string]any{"admin_assignee": map[string]any{"name": "Riley"}}
	if got := normalizeAssignee(raw); got != "Riley" {
		t.Fatalf("admin name = %q", got)
	}
	raw = map[string]any{"admin_assignee": map[string]any{"id": "adm_1"}}
	if got := normalizeAssignee(raw); got != "adm_1" {
		t.Fatalf("admin id = %q", got)
	}
	raw = map[string]any{"team_assignee_id": "team_42"}
	if got := normalizeAssignee(raw); got != "team_42" {
		t.Fatalf("team id = %q", got)
	}
	if got := normalizeAssignee(map[string]any{}); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

func TestNormalizeRatingPrefersScoreThenRemark(t *testing.T) {
	raw := map[string]any{"conversation_rating": map[string]any{"rating": "5"}}
	if got := normalizeRating(raw); got != "5" {
		t.Fatalf("score = %q", got)
	}
	raw = map[string]any{"conversation_rating": map[string]any{"remark": "great"}}
	if got := normalizeRating(raw); got != "great" {
		t.Fatalf("remark = %q", got)
	}
	if got := normalizeRating(map[string]any{}); got != "" {
		t.Fatalf("absent = %q", got)
	}
}

func TestNormalizeFinStatusReadsCanonicalAndLegacy(t *testing.T) {
	if got := normalizeFinStatus(map[string]any{"fin_status": "resolved"}); got != "resolved" {
		t.Fatalf("fin_status = %q", got)
	}
	if got := normalizeFinStatus(map[string]any{"ai_agent_status": "handed_over"}); got != "handed_over" {
		t.Fatalf("ai_agent_status = %q", got)
	}
	if got := normalizeFinStatus(map[string]any{"ai_agent_participated": true}); got != "participated" {
		t.Fatalf("participated = %q", got)
	}
	if got := normalizeFinStatus(map[string]any{"ai_agent_participated": false}); got != "" {
		t.Fatalf("not participated = %q", got)
	}
}

func TestConversationSubjectFallsBackThroughLayers(t *testing.T) {
	if got := conversationSubject(map[string]any{"title": "Title here"}, nil); got != "Title here" {
		t.Fatalf("title = %q", got)
	}
	if got := conversationSubject(map[string]any{"subject": "Subject here"}, nil); got != "Subject here" {
		t.Fatalf("subject = %q", got)
	}
	source := map[string]any{"source": map[string]any{"subject": "From source"}}
	if got := conversationSubject(source, nil); got != "From source" {
		t.Fatalf("source subject = %q", got)
	}
	if got := conversationSubject(map[string]any{}, []store.Part{{Body: "hello there friend goodbye"}}); got != "hello there friend goodbye" {
		t.Fatalf("part-derived subject = %q", got)
	}
	if got := conversationSubject(map[string]any{}, nil); got != "Intercom conversation" {
		t.Fatalf("default = %q", got)
	}
}

func TestStringValueAcceptsStringAndNumeric(t *testing.T) {
	if got := stringValue(map[string]any{"k": " trimmed "}, "k"); got != "trimmed" {
		t.Fatalf("trimmed = %q", got)
	}
	if got := stringValue(map[string]any{"k": float64(42)}, "k"); got != "42" {
		t.Fatalf("int float = %q", got)
	}
	if got := stringValue(map[string]any{"k": float64(1.5)}, "k"); got != "1.5" {
		t.Fatalf("frac float = %q", got)
	}
	if got := stringValue(map[string]any{"k": true}, "k"); got != "" {
		t.Fatalf("bool = %q", got)
	}
}

func TestTimeValueHandlesStringAndUnix(t *testing.T) {
	if got := timeValue(map[string]any{"k": "2026-05-19T01:02:03Z"}, "k"); got != "2026-05-19T01:02:03Z" {
		t.Fatalf("rfc3339 = %q", got)
	}
	if got := timeValue(map[string]any{"k": "not a date"}, "k"); got != "not a date" {
		t.Fatalf("passthrough = %q", got)
	}
	if got := timeValue(map[string]any{"k": float64(0)}, "k"); got != "1970-01-01T00:00:00Z" {
		t.Fatalf("unix epoch = %q", got)
	}
	if got := timeValue(map[string]any{"k": "  "}, "k"); got != "" {
		t.Fatalf("blank = %q", got)
	}
	if got := timeValue(map[string]any{"k": true}, "k"); got != "" {
		t.Fatalf("bool = %q", got)
	}
}

func TestAuthorNamePicksFirstNonEmpty(t *testing.T) {
	if got := authorName(map[string]any{"author": map[string]any{"name": "Riley", "email": "r@example.com"}}); got != "Riley" {
		t.Fatalf("name = %q", got)
	}
	if got := authorName(map[string]any{"author": map[string]any{"email": "r@example.com"}}); got != "r@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := authorName(map[string]any{"author": map[string]any{"type": "admin"}}); got != "admin" {
		t.Fatalf("type = %q", got)
	}
	if got := authorName(map[string]any{}); got != "" {
		t.Fatalf("none = %q", got)
	}
}

func TestIntercomSyncerSleepUsesClientHookAndDefault(t *testing.T) {
	called := false
	s := IntercomSyncer{Client: intercom.Client{Sleep: func(ctx context.Context, d time.Duration) error {
		called = true
		return nil
	}}}
	if err := s.sleep(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatalf("expected client sleep hook to be called")
	}
	// Default path with very short delay should return promptly.
	bare := IntercomSyncer{}
	if err := bare.sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("default sleep: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bare.sleep(ctx, time.Hour); err == nil {
		t.Fatalf("cancelled context should error")
	}
}

func TestIsScopeDenied(t *testing.T) {
	if isScopeDenied(nil) {
		t.Fatalf("nil should not be scope-denied")
	}
	if isScopeDenied(io.EOF) {
		t.Fatalf("non-status error should not be scope-denied")
	}
	if !isScopeDenied(intercom.HTTPStatusError{StatusCode: 401}) {
		t.Fatalf("401 should be scope-denied")
	}
	if !isScopeDenied(intercom.HTTPStatusError{StatusCode: 403}) {
		t.Fatalf("403 should be scope-denied")
	}
	if isScopeDenied(intercom.HTTPStatusError{StatusCode: 500}) {
		t.Fatalf("500 should not be scope-denied")
	}
}
