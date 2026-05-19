package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/uinaf/fincrawl/internal/config"
)

func newIntercomMock(t *testing.T) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var searchCalls int32
	var conversationCalls int32
	conversationJSON := `{
		"id": "conv_live_1",
		"title": "Synthetic live thread",
		"state": "open",
		"created_at": 1770000000,
		"updated_at": 1770000300,
		"source": {"id": "source_live_1", "body": "live body", "author": {"name": "Pat Live"}},
		"conversation_parts": {"conversation_parts": [{"id": "part_live_1", "part_type": "comment", "body": "live reply", "created_at": 1770000100, "author": {"name": "Riley Live"}}]},
		"tags": {"tags": [{"name": "billing"}]}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/admins":
			fmt.Fprint(w, `{"admins":[{"id":"admin_live_1","name":"Riley Live","email":"riley@example.invalid","team_ids":["team_live_1"]}]}`)
		case r.URL.Path == "/teams":
			fmt.Fprint(w, `{"teams":[{"id":"team_live_1","name":"Live Support"}]}`)
		case r.URL.Path == "/tags":
			fmt.Fprint(w, `{"tags":[{"id":"tag_live_1","name":"billing"}]}`)
		case r.URL.Path == "/contacts":
			fmt.Fprint(w, `{"data":[{"id":"contact_live_1","name":"Casey Live"}],"pages":{"next":{}}}`)
		case r.URL.Path == "/conversations/search":
			atomic.AddInt32(&searchCalls, 1)
			fmt.Fprint(w, `{"conversations":[{"id":"conv_live_1","updated_at":1770000300}],"pages":{"next":{}}}`)
		case strings.HasPrefix(r.URL.Path, "/conversations/"):
			atomic.AddInt32(&conversationCalls, 1)
			fmt.Fprint(w, conversationJSON)
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, &searchCalls, &conversationCalls
}

func TestLiveSyncEntities(t *testing.T) {
	server, _, _ := newIntercomMock(t)
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	t.Setenv(config.EnvIntercomBase, server.URL)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--entities", "--contacts", "--limit", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("sync --entities: %v\nstderr=%s", err, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout.String())
	}
}

func TestLiveSyncSingleConversation(t *testing.T) {
	server, _, conversationCalls := newIntercomMock(t)
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	t.Setenv(config.EnvIntercomBase, server.URL)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--conversation", "conv_live_1", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("sync --conversation: %v\nstderr=%s", err, stderr.String())
	}
	if atomic.LoadInt32(conversationCalls) == 0 {
		t.Fatalf("expected at least one /conversations/{id} call, got 0")
	}
}

func TestLiveSyncUpdatedSinceAndResume(t *testing.T) {
	server, searchCalls, _ := newIntercomMock(t)
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	t.Setenv(config.EnvIntercomBase, server.URL)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--updated-since", "2h", "--limit", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("sync --updated-since: %v\nstderr=%s", err, stderr.String())
	}
	if atomic.LoadInt32(searchCalls) == 0 {
		t.Fatalf("expected at least one search call")
	}
	// Now a resume with no active window should fail because window completed.
	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"sync", "--resume", "--limit", "5", "--json"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected resume to fail with no active window")
	}
}

func TestLiveSyncDryRunReportsPlanWithoutContactingProvider(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--updated-since", "1h", "--updated-before", "30m", "--limit", "5", "--dry-run", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("sync --dry-run: %v\nstderr=%s", err, stderr.String())
	}
	var plan map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, stdout.String())
	}
	if plan["mode"] == nil {
		t.Fatalf("plan missing mode: %#v", plan)
	}
}

func TestLiveSyncRequiresTokenWhenNotDryRun(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--updated-since", "1h"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected missing-token failure")
	}
}
