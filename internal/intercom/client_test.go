package intercom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchConversationsUsesSearchCursorAndVersion(t *testing.T) {
	var sawVersion string
	var sawCursor string
	var sawQuery []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		sawVersion = r.Header.Get("Intercom-Version")
		var body struct {
			Query struct {
				Value []map[string]any `json:"value"`
			} `json:"query"`
			Pagination map[string]any `json:"pagination"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sawQuery = body.Query.Value
		sawCursor, _ = body.Pagination["starting_after"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"conversations":[{"id":"conversation_1","updated_at":1770000000}],"pages":{"next":{"starting_after":"cursor_2"}}}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, Token: "test-token", Version: "2.13", HTTPClient: server.Client()}
	result, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), "cursor_1")
	if err != nil {
		t.Fatal(err)
	}
	if sawVersion != "2.13" {
		t.Fatalf("version = %q", sawVersion)
	}
	if sawCursor != "cursor_1" {
		t.Fatalf("cursor = %q", sawCursor)
	}
	if len(sawQuery) != 2 || sawQuery[0]["value"] != float64(0) || sawQuery[1]["value"] != float64(3) {
		t.Fatalf("query bounds = %#v, want strict operators around inclusive seconds", sawQuery)
	}
	if result.NextCursor != "cursor_2" {
		t.Fatalf("next cursor = %q", result.NextCursor)
	}
}

func TestSearchConversationsDefaultsAPIVersion(t *testing.T) {
	var sawVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawVersion = r.Header.Get("Intercom-Version")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"conversations":[],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), ""); err != nil {
		t.Fatal(err)
	}
	if sawVersion != DefaultAPIVersion {
		t.Fatalf("version = %q, want %q", sawVersion, DefaultAPIVersion)
	}
}

func TestRetrieveConversationUsesPlaintextDisplay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/abc123" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("display_as"); got != "plaintext" {
			t.Fatalf("display_as = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"abc123","conversation_parts":{"conversation_parts":[]}}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	conversation, err := client.RetrieveConversation(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.ID != "abc123" || len(conversation.Raw) == 0 {
		t.Fatalf("unexpected conversation: %#v", conversation)
	}
}

func TestRetrieveConversationEscapesPathSegments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/conversations/abc%2Fdef" {
			t.Fatalf("escaped path = %s", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"abc/def","conversation_parts":{"conversation_parts":[]}}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.RetrieveConversation(context.Background(), "abc/def"); err != nil {
		t.Fatal(err)
	}
}

func TestListEntitiesDecodesWorkspaceMetadata(t *testing.T) {
	var sawPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPaths = append(sawPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admins":
			w.Write([]byte(`{"admins":[{"id":"admin_syn_1","name":"Riley Example","email":"riley@example.invalid","team_ids":["team_syn_1"]}]}`))
		case "/teams":
			w.Write([]byte(`{"teams":[{"id":"team_syn_1","name":"Synthetic Support"}]}`))
		case "/tags":
			w.Write([]byte(`{"tags":[{"id":"tag_syn_1","name":"billing"}]}`))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	admins, err := client.ListAdmins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	teams, err := client.ListTeams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tags, err := client.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 1 || admins[0].ID != "admin_syn_1" || admins[0].TeamIDs[0] != "team_syn_1" {
		t.Fatalf("admins = %#v", admins)
	}
	if len(teams) != 1 || teams[0].Name != "Synthetic Support" {
		t.Fatalf("teams = %#v", teams)
	}
	if len(tags) != 1 || tags[0].Name != "billing" {
		t.Fatalf("tags = %#v", tags)
	}
	if len(sawPaths) != 3 {
		t.Fatalf("paths = %#v", sawPaths)
	}
}

func TestListContactsCapsPaginatedReads(t *testing.T) {
	var requests int
	var perPages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		perPages = append(perPages, r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		switch requests {
		case 1:
			w.Write([]byte(`{"data":[{"id":"contact_syn_1","name":"Casey Example"}],"pages":{"next":{"starting_after":"cursor_2"}}}`))
		case 2:
			if r.URL.Query().Get("starting_after") != "cursor_2" {
				t.Fatalf("starting_after = %q", r.URL.Query().Get("starting_after"))
			}
			w.Write([]byte(`{"data":[{"id":"contact_syn_2","email":"jordan@example.invalid"}],"pages":{"next":{}}}`))
		default:
			t.Fatalf("unexpected contact request %d", requests)
		}
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	contacts, err := client.ListContacts(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 || contacts[1].Name != "jordan@example.invalid" {
		t.Fatalf("contacts = %#v", contacts)
	}
	if perPages[0] != "2" || perPages[1] != "1" {
		t.Fatalf("per_page values = %#v", perPages)
	}
}

func TestRateLimitHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), "")
	rateErr, ok := err.(RateLimitError)
	if !ok {
		t.Fatalf("err = %T, want RateLimitError", err)
	}
	if rateErr.RetryAfter != 3*time.Second {
		t.Fatalf("retry after = %s", rateErr.RetryAfter)
	}
}

func TestRateLimitHandlingUsesResetHeader(t *testing.T) {
	now := time.Unix(100, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Reset", "105")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	_, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), "")
	rateErr, ok := err.(RateLimitError)
	if !ok {
		t.Fatalf("err = %T, want RateLimitError", err)
	}
	if rateErr.RetryAfter != 5*time.Second {
		t.Fatalf("retry after = %s", rateErr.RetryAfter)
	}
}

func TestSearchConversationsRetriesTransientServerErrors(t *testing.T) {
	var requests int
	var slept time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			http.Error(w, "try again", http.StatusBadGateway)
			return
		}
		w.Write([]byte(`{"conversations":[{"id":"conversation_1","updated_at":1770000000}],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	client := Client{
		BaseURL:      server.URL,
		HTTPClient:   server.Client(),
		MaxAttempts:  2,
		RetryBackoff: 7 * time.Millisecond,
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = d
			return nil
		},
	}
	result, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), "")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if slept != 7*time.Millisecond {
		t.Fatalf("sleep = %s, want 7ms", slept)
	}
	if len(result.Conversations) != 1 {
		t.Fatalf("conversations = %#v", result.Conversations)
	}
}

func TestLowRemainingBudgetSleepsUntilReset(t *testing.T) {
	now := time.Unix(100, 0)
	var slept time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "1")
		w.Header().Set("X-RateLimit-Reset", "104")
		w.Write([]byte(`{"conversations":[],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	client := Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Now:        func() time.Time { return now },
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = d
			return nil
		},
		ThrottleBelow: 2,
	}
	if _, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), ""); err != nil {
		t.Fatal(err)
	}
	if slept != 4*time.Second {
		t.Fatalf("sleep = %s, want 4s", slept)
	}
}

func TestRateLimitErrorString(t *testing.T) {
	if got := (RateLimitError{}).Error(); got != "intercom rate limited" {
		t.Fatalf("zero = %q", got)
	}
	if got := (RateLimitError{RetryAfter: 3 * time.Second}).Error(); got != "intercom rate limited: retry after 3s" {
		t.Fatalf("with retry = %q", got)
	}
}

func TestHTTPStatusErrorString(t *testing.T) {
	bare := HTTPStatusError{Method: "GET", Path: "/conversations/1", StatusCode: 404}
	if got := bare.Error(); got != "intercom GET /conversations/1 failed: status 404" {
		t.Fatalf("bare = %q", got)
	}
	withBody := HTTPStatusError{Method: "POST", Path: "/search", StatusCode: 500, Body: "oops"}
	if got := withBody.Error(); got != "intercom POST /search failed: status 500: oops" {
		t.Fatalf("with body = %q", got)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string { return "i/o timeout" }
func (timeoutErr) Timeout() bool { return true }

type nonTimeoutErr struct{}

func (nonTimeoutErr) Error() string { return "boom" }
func (nonTimeoutErr) Timeout() bool { return false }

func TestShouldRetryError(t *testing.T) {
	if !shouldRetryError(timeoutErr{}) {
		t.Fatalf("timeoutErr should retry")
	}
	if shouldRetryError(nonTimeoutErr{}) {
		t.Fatalf("nonTimeoutErr should not retry")
	}
	if shouldRetryError(nil) {
		t.Fatalf("nil should not retry")
	}
	plain := errorString("plain")
	if shouldRetryError(plain) {
		t.Fatalf("plain string err should not retry")
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestRetryDelayScales(t *testing.T) {
	if d := retryDelay(0, 0); d != time.Second {
		t.Fatalf("zero base zero attempt = %s", d)
	}
	if d := retryDelay(2*time.Second, 1); d != 2*time.Second {
		t.Fatalf("attempt 1 = %s", d)
	}
	if d := retryDelay(2*time.Second, 3); d != 6*time.Second {
		t.Fatalf("attempt 3 = %s", d)
	}
}

func TestLowRemainingBudgetThrottlesSuccessfulResponse(t *testing.T) {
	var slept bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "1")
		w.Write([]byte(`{"conversations":[],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	client := Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = true
			return nil
		},
		ThrottleBelow: 2,
	}
	if _, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), ""); err != nil {
		t.Fatal(err)
	}
	if !slept {
		t.Fatalf("expected throttle sleep")
	}
}

func TestStringValueIntercomTypes(t *testing.T) {
	if got := stringValue(map[string]any{"k": "  hi  "}, "k"); got != "hi" {
		t.Fatalf("string = %q", got)
	}
	if got := stringValue(map[string]any{"k": float64(7)}, "k"); got != "7" {
		t.Fatalf("int float = %q", got)
	}
	if got := stringValue(map[string]any{"k": float64(1.5)}, "k"); got != "1.5" {
		t.Fatalf("frac float = %q", got)
	}
	if got := stringValue(map[string]any{"k": true}, "k"); got != "" {
		t.Fatalf("bool = %q", got)
	}
	if got := stringValue(map[string]any{}, "missing"); got != "" {
		t.Fatalf("missing = %q", got)
	}
}

func TestRawItemsAcceptsArrayAndKeyed(t *testing.T) {
	arr, err := rawItems([]byte(`[{"id":"a"},{"id":"b"}]`), "data")
	if err != nil || len(arr) != 2 {
		t.Fatalf("array = %v %d err=%v", arr, len(arr), err)
	}
	keyed, err := rawItems([]byte(`{"data":[{"id":"a"}]}`), "data")
	if err != nil || len(keyed) != 1 {
		t.Fatalf("keyed = %v %d err=%v", keyed, len(keyed), err)
	}
	if _, err := rawItems([]byte(`{"unrelated":1}`), "data", "alt"); err == nil {
		t.Fatalf("expected missing-keys error")
	}
	if _, err := rawItems([]byte(`not-json`), "data"); err == nil {
		t.Fatalf("expected json error")
	}
	if _, err := rawItems([]byte(`{"data":"notalist"}`), "data"); err == nil {
		t.Fatalf("expected list decode error")
	}
}

func TestNextCursorReadsPaging(t *testing.T) {
	if got := nextCursor([]byte(`{"pages":{"next":{"starting_after":"cursor_42"}}}`)); got != "cursor_42" {
		t.Fatalf("cursor = %q", got)
	}
	if got := nextCursor([]byte(`{}`)); got != "" {
		t.Fatalf("missing cursor = %q", got)
	}
	if got := nextCursor([]byte(`not json`)); got != "" {
		t.Fatalf("bad json = %q", got)
	}
}

func TestSleepHonoursContextAndDefaultTimer(t *testing.T) {
	c := Client{}
	if err := c.sleep(context.Background(), 0); err != nil {
		t.Fatalf("zero delay = %v", err)
	}
	if err := c.sleep(context.Background(), -time.Second); err != nil {
		t.Fatalf("negative delay = %v", err)
	}
	if err := c.sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("short timer = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.sleep(ctx, time.Hour); err == nil {
		t.Fatalf("cancelled context should error")
	}
}

func TestFirstNonEmptyHelper(t *testing.T) {
	if got := firstNonEmpty("", "  ", "hit", "next"); got != "hit" {
		t.Fatalf("hit = %q", got)
	}
	if got := firstNonEmpty("", "", " "); got != "" {
		t.Fatalf("all empty = %q", got)
	}
}

func TestDoJSONReturnsHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client(), MaxAttempts: 1}
	_, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), "")
	statusErr, ok := err.(HTTPStatusError)
	if !ok {
		t.Fatalf("err = %T %v, want HTTPStatusError", err, err)
	}
	if statusErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", statusErr.StatusCode)
	}
}

func TestDoJSONRetriesOn5xx(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "transient", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"conversations":[],"pages":{"next":{}}}`))
	}))
	defer server.Close()
	client := Client{
		BaseURL:     server.URL,
		HTTPClient:  server.Client(),
		MaxAttempts: 3,
		Sleep:       func(ctx context.Context, d time.Duration) error { return nil },
	}
	if _, err := client.SearchConversations(context.Background(), time.Unix(1, 0), time.Unix(2, 0), ""); err != nil {
		t.Fatal(err)
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d, expected retry", attempts)
	}
}
