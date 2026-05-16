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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		sawVersion = r.Header.Get("Intercom-Version")
		var body struct {
			Pagination map[string]any `json:"pagination"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
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
