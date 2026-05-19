package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/openclaw/crawlkit/output"
	"github.com/uinaf/fincrawl/internal/config"
	"github.com/uinaf/fincrawl/internal/store"
)

func TestSyncRejectsAmbiguousModes(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--fixture", "testdata/synthetic",
		"--conversation", "conversation_synthetic",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsMissingMode(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"sync"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsNegativeUpdatedSinceLimit(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--updated-since", "2h",
		"--limit=-1",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsUpdatedBeforeWithoutUpdatedSince(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--updated-before", "90d",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsInvertedUpdatedWindow(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--updated-since", "2026-05-17T12:00:00Z",
		"--updated-before", "2026-05-17T11:00:00Z",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncUpdatedSinceDryRunAcceptsBoundedWindow(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--updated-since", "2026-02-17T00:00:00Z",
		"--updated-before", "2026-05-17T00:00:00Z",
		"--dry-run",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Mode       string         `json:"mode"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "updated-since" {
		t.Fatalf("mode = %q", plan.Mode)
	}
	if plan.Parameters["updated_after"] != "2026-02-17T00:00:00Z" {
		t.Fatalf("updated_after = %#v", plan.Parameters["updated_after"])
	}
	if plan.Parameters["updated_before"] != "2026-05-17T00:00:00Z" {
		t.Fatalf("updated_before = %#v", plan.Parameters["updated_before"])
	}
	if plan.Parameters["updated_before_input"] != "2026-05-17T00:00:00Z" {
		t.Fatalf("updated_before_input = %#v", plan.Parameters["updated_before_input"])
	}
}

func TestSyncRejectsNegativeResumeLimit(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--resume",
		"--limit=-1",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsContactsWithoutEntities(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"sync", "--contacts"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncEntitiesRequiresLiveCredential(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"sync", "--entities", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), config.EnvIntercomCred) {
		t.Fatalf("expected missing credential error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncResumeReachesLiveDispatch(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"sync", "--resume", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "no active Intercom tail sync state") {
		t.Fatalf("expected missing active state error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsUnsafeConversationID(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"sync", "--conversation", "../conv?x=1", "--json"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestArchiveRejectsAbsoluteOutput(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join("..", "..", "testdata", "synthetic"),
		"--recipient", "age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl",
		"--out", filepath.Join(t.TempDir(), "snapshot.jsonl.zst.age"),
		"--json",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestDescribeSearchPrintsSchema(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"describe", "search", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"schema_version": "fincrawl.cli.v1"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"fields"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"fin-status"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSearchFieldsProjectsResults(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--fields", "provider_id,subject", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"provider_id": "ic_syn_002"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"snippet"`)) {
		t.Fatalf("field mask leaked snippet: %q", stdout.String())
	}
}

func TestSearchFiltersResults(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "invoice", "--state", "open", "--tag", "billing", "--fields", "provider_id,state,tags", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"provider_id": "ic_syn_001"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "invoice", "--state", "closed", "--fields", "provider_id", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "[]" {
		t.Fatalf("stdout = %q, want empty results", stdout.String())
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "login", "--fin-status", "resolved", "--fields", "provider_id,fin_status", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"fin_status": "resolved"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSearchNDJSONStreamsProjectedResults(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--fields", "provider_id,subject", "--ndjson"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, stdout = %q", len(lines), stdout.String())
	}
	if !strings.Contains(lines[0], `"provider_id":"ic_syn_002"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if strings.Contains(lines[0], `"snippet"`) {
		t.Fatalf("field mask leaked snippet: %q", stdout.String())
	}
}

func TestShowConversationDefaultsToNoParts(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"show", "ic_syn_002", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["provider_id"] != "ic_syn_002" {
		t.Fatalf("result = %#v", result)
	}
	if _, ok := result["parts"]; ok {
		t.Fatalf("default show leaked parts: %q", stdout.String())
	}
}

func TestShowConversationPartsAreOptIn(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"show", "ic_syn_002", "--parts", "--part-limit", "1", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		ProviderID string `json:"provider_id"`
		Parts      []struct {
			Body string `json:"body"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ProviderID != "ic_syn_002" || len(result.Parts) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if strings.ContainsAny(result.Parts[0].Body, "\x00\n\t") {
		t.Fatalf("unsafe body = %q", result.Parts[0].Body)
	}
}

func TestDescribePublishesExitCodes(t *testing.T) {
	want := map[string]int{"ok": 0, "runtime_error": 1, "usage_error": 2}

	for _, args := range [][]string{
		{"describe", "--json"},
		{"describe", "search", "--json"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var payload struct {
			ExitCodes []struct {
				Name string `json:"name"`
				Code int    `json:"code"`
			} `json:"exit_codes"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("%v unmarshal: %v\nstdout = %q", args, err, stdout.String())
		}
		if len(payload.ExitCodes) != len(want) {
			t.Fatalf("%v exit_codes len = %d, want %d (payload = %+v)", args, len(payload.ExitCodes), len(want), payload.ExitCodes)
		}
		got := map[string]int{}
		for _, c := range payload.ExitCodes {
			got[c.Name] = c.Code
		}
		for name, code := range want {
			if g, ok := got[name]; !ok || g != code {
				t.Fatalf("%v exit_codes[%q] = %d (present=%v), want %d", args, name, g, ok, code)
			}
		}
	}
}

func TestDescribeStoreVerifyAcceptsCommandWords(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"describe", "store", "verify", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"store verify"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSearchFieldsRejectsUnknownFieldWithoutHits(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	err := Run(context.Background(), []string{"search", "zzzunlikely", "--fields", "nope", "--json"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStoreVerifyAcceptsEncryptedManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "synthetic.jsonl.zst.age"), []byte("synthetic encrypted placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/synthetic.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"store", "verify", root, "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok": true`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStoreVerifyRejectsPlaintextArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "synthetic.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":["snapshots/synthetic.jsonl"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"store", "verify", root, "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "tenant store verification failed") {
		t.Fatalf("expected tenant store verification error, got %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`plaintext archive artifacts`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSearchFieldsRejectsUnknownFieldBeforeStoreOpen(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"search", "anything", "--fields", "nope", "--json"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestWriteErrorJSONEnvelope(t *testing.T) {
	var stderr bytes.Buffer

	exitCode := WriteError(&stderr, output.UsageError{Err: fmt.Errorf("bad input")}, []string{"sync"})
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Usage   bool   `json:"usage"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error.Code != "usage_error" || !envelope.Error.Usage || envelope.Error.Message != "bad input" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestWriteErrorTextOptOut(t *testing.T) {
	var stderr bytes.Buffer

	exitCode := WriteError(&stderr, output.UsageError{Err: fmt.Errorf("bad input")}, []string{"sync", "--json=false"})
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if got := strings.TrimSpace(stderr.String()); got != "bad input" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestWriteErrorJSONEnvelopeAcceptsAssignedJSONFlag(t *testing.T) {
	var stderr bytes.Buffer

	exitCode := WriteError(&stderr, output.UsageError{Err: fmt.Errorf("bad input")}, []string{"sync", "--json=true"})
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`"code": "usage_error"`)) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHelpPrintsUsageAndSucceeds(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
	}{
		{
			name: "root",
			args: []string{"--help"},
			contains: []string{
				"Usage: fincrawl <command>",
				"Local-first support conversation archive.",
				"sync",
			},
		},
		{
			name: "subcommand",
			args: []string{"sync", "--help"},
			contains: []string{
				"Usage: fincrawl sync",
				"--fixture=STRING",
				"--updated-since=STRING",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("expected help to succeed, got %v", err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
			for _, want := range tt.contains {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
			if strings.Contains(stdout.String(), `"ok": false`) {
				t.Fatalf("stdout contains error envelope: %q", stdout.String())
			}
		})
	}
}

func TestParserErrorsAreUsageErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{"search", "--json=true"}
	err := Run(context.Background(), args, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	stderr.Reset()
	exitCode := WriteError(&stderr, err, args)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`"usage": true`)) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestVersionPrintsJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"version", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"version": "dev"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncFixtureDryRunDoesNotCreateDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	rt, err := config.LoadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err = Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic"), "--dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		DryRun      bool `json:"dry_run"`
		WouldMutate bool `json:"would_mutate"`
		Counts      struct {
			Conversations int `json:"conversations"`
		} `json:"counts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || !plan.WouldMutate || plan.Counts.Conversations == 0 {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := os.Stat(rt.Config.DBPath); !os.IsNotExist(err) {
		t.Fatalf("dry run created database or unexpected stat error: %v", err)
	}
}

func TestArchiveDryRunDoesNotWriteArtifact(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	out := filepath.Join("tmp", "archive-dry-run-test.jsonl.zst.age")
	t.Cleanup(func() {
		_ = os.Remove(out)
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join("..", "..", "testdata", "synthetic"),
		"--recipient", "age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl",
		"--out", out,
		"--dry-run",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		DryRun      bool   `json:"dry_run"`
		WouldMutate bool   `json:"would_mutate"`
		Output      string `json:"output"`
		Records     int    `json:"records"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || !plan.WouldMutate || plan.Output != out || plan.Records == 0 {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote artifact or unexpected stat error: %v", err)
	}
}

func TestPublishImportEncryptedSnapshotRoundTrip(t *testing.T) {
	sourceHome := t.TempDir()
	targetHome := t.TempDir()
	t.Setenv("FINCRAWL_HOME", sourceHome)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("tmp", fmt.Sprintf("publish-import-%d.jsonl.zst.age", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = os.Remove(out)
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"publish", "--recipient", identity.Recipient().String(), "--out", out}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("published artifact missing: %v", err)
	}
	stdout.Reset()
	if err := os.Setenv("FINCRAWL_HOME", targetHome); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"import", "--identity", identity.String(), "--in", out}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--fields", "provider_id,subject", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"provider_id": "ic_syn_002"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSubscribeDryRunVerifiesLocalStoreWithoutIdentity(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "store")
	if err := os.MkdirAll(filepath.Join(storeRoot, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "snapshots", "synthetic.jsonl.zst.age"), []byte("encrypted placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "manifest.json"), []byte(`{"snapshots":[{"path":" snapshots/synthetic.jsonl.zst.age ","records":13}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"subscribe", storeRoot, "--dry-run", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		DryRun            bool `json:"dry_run"`
		ImportedSnapshots int  `json:"imported_snapshots"`
		Records           int  `json:"records"`
		Snapshots         []struct {
			Path    string `json:"path"`
			Records int    `json:"records"`
		} `json:"snapshots"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.ImportedSnapshots != 0 || result.Records != 13 || len(result.Snapshots) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Snapshots[0].Path != "snapshots/synthetic.jsonl.zst.age" {
		t.Fatalf("snapshot path = %q", result.Snapshots[0].Path)
	}
}

func TestSubscribeImportsLocalTenantStore(t *testing.T) {
	sourceHome := t.TempDir()
	targetHome := t.TempDir()
	t.Setenv("FINCRAWL_HOME", sourceHome)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := filepath.Join("tmp", fmt.Sprintf("subscribe-store-%d", time.Now().UnixNano()))
	out := filepath.Join(storeRoot, "snapshots", "synthetic.jsonl.zst.age")
	t.Cleanup(func() {
		_ = os.RemoveAll(storeRoot)
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"publish", "--recipient", identity.Recipient().String(), "--out", out}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/synthetic.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := os.Setenv("FINCRAWL_HOME", targetHome); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"subscribe", storeRoot, "--identity", identity.String(), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		ImportedSnapshots int `json:"imported_snapshots"`
		Records           int `json:"records"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ImportedSnapshots != 1 || result.Records == 0 {
		t.Fatalf("result = %#v", result)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--fields", "provider_id,subject", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"provider_id": "ic_syn_002"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestParseSinceAcceptsDayDurations(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	got, err := parseSince("2d", now, "updated-since")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("since = %s, want %s", got, want)
	}
}

func TestParseConversationFieldsAcceptsKnownNames(t *testing.T) {
	names, err := parseConversationFields("provider_id, subject ,snippet, parts ")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"provider_id", "subject", "snippet", "parts"}
	if len(names) != len(want) {
		t.Fatalf("names = %#v want %#v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("names[%d] = %q, want %q", i, names[i], n)
		}
	}
}

func TestParseConversationFieldsRejectsBlanksAndUnknowns(t *testing.T) {
	if _, err := parseConversationFields("provider_id,,subject"); err == nil {
		t.Fatalf("expected empty-field error")
	}
	if _, err := parseConversationFields("provider_id,not_a_field"); err == nil {
		t.Fatalf("expected unknown-field error")
	}
}

func TestValidateConversationFieldsAcceptsEmpty(t *testing.T) {
	if err := validateConversationFields("   "); err != nil {
		t.Fatalf("blank rejected: %v", err)
	}
	if err := validateConversationFields("provider_id"); err != nil {
		t.Fatalf("provider_id rejected: %v", err)
	}
}

func TestImportDryRunShapesPlan(t *testing.T) {
	plan := importDryRun("snapshots/in.jsonl.zst.age", "/tmp/db", 42)
	if plan.Command != "import" || !plan.DryRun || !plan.WouldMutate {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Input != "snapshots/in.jsonl.zst.age" || plan.DatabasePath != "/tmp/db" || plan.Records != 42 {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.WouldRead) != 1 || plan.WouldRead[0] != "snapshots/in.jsonl.zst.age" {
		t.Fatalf("would_read = %#v", plan.WouldRead)
	}
	if len(plan.WouldWrite) != 1 || plan.WouldWrite[0] != "/tmp/db" {
		t.Fatalf("would_write = %#v", plan.WouldWrite)
	}
}

func TestRedactPresence(t *testing.T) {
	if got := redactPresence(true); got != "present" {
		t.Fatalf("present = %q", got)
	}
	if got := redactPresence(false); got != "absent" {
		t.Fatalf("absent = %q", got)
	}
}

func TestFormatOptionalTime(t *testing.T) {
	if got := formatOptionalTime(time.Time{}); got != "" {
		t.Fatalf("zero = %q", got)
	}
	tm := time.Date(2026, 5, 19, 1, 2, 3, 0, time.UTC)
	if got := formatOptionalTime(tm); got != "2026-05-19T01:02:03Z" {
		t.Fatalf("time = %q", got)
	}
}

func TestValidateProviderIDRejectsBadShapes(t *testing.T) {
	if err := validateProviderID("conversation", ""); err == nil {
		t.Fatalf("expected error for empty id")
	}
	if err := validateProviderID("conversation", "  surrounded  "); err == nil {
		t.Fatalf("expected error for whitespace surround")
	}
	long := strings.Repeat("a", 257)
	if err := validateProviderID("conversation", long); err == nil {
		t.Fatalf("expected error for overlong id")
	}
	if err := validateProviderID("conversation", "has space"); err == nil {
		t.Fatalf("expected error for whitespace inside")
	}
	if err := validateProviderID("conversation", "has/slash"); err == nil {
		t.Fatalf("expected error for slash")
	}
	if err := validateProviderID("conversation", "has?query"); err == nil {
		t.Fatalf("expected error for query char")
	}
	if err := validateProviderID("conversation", "%2e%2e"); err == nil {
		t.Fatalf("expected error for percent-encoded traversal")
	}
	if err := validateProviderID("conversation", "good_id-123"); err != nil {
		t.Fatalf("good id rejected: %v", err)
	}
}

func TestDoctorOfflinePrintsJSONResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Setenv(config.EnvAgeRecipient, "")
	t.Setenv(config.EnvAgeIdentity, "")
	t.Setenv(config.EnvIntercomCred, "")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor", "--offline", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	var payload struct {
		OK          bool `json:"ok"`
		Offline     bool `json:"offline"`
		Credentials map[string]string
		Checks      []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v\nstdout = %q", err, stdout.String())
	}
	if !payload.OK || !payload.Offline {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Credentials[config.EnvAgeRecipient] != "absent" {
		t.Fatalf("recipient not redacted as absent: %+v", payload.Credentials)
	}
	if len(payload.Checks) == 0 {
		t.Fatalf("checks missing: %+v", payload.Checks)
	}
}

func TestMetadataPrintsManifest(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"metadata", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"id": "fincrawl"`)) {
		t.Fatalf("metadata missing id: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"capabilities"`)) {
		t.Fatalf("metadata missing capabilities: %s", stdout.String())
	}
}

func TestVersionPrintsBuildInfo(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"version", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"version"`)) {
		t.Fatalf("version missing version field: %s", stdout.String())
	}
}

func TestGuardJSONReportsScannedAndOK(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(wd, "..", ".."))
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"guard", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok": true`)) {
		t.Fatalf("guard payload: %s", stdout.String())
	}
}

func TestStatusOnEmptyHomeIsEmpty(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"status", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"state"`)) {
		t.Fatalf("status missing state: %s", stdout.String())
	}
}

func TestSearchResultFieldCoversAllNames(t *testing.T) {
	res := store.SearchResult{
		ID: "1", ProviderID: "ic_1", Subject: "subj", State: "open", Assignee: "Riley",
		Rating: "5", FinStatus: "resolved", Participants: []string{"a"}, Tags: []string{"t"},
		UpdatedAt: "2026-05-19T00:00:00Z", Snippet: "snip", Score: 1.5,
	}
	for _, name := range []string{"id", "provider_id", "subject", "state", "assignee", "rating", "fin_status", "participants", "tags", "updated_at", "snippet", "score"} {
		if _, err := searchResultField(res, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := searchResultField(res, "bogus"); err == nil {
		t.Fatalf("expected unknown field error")
	}
}

func TestConversationDetailFieldCoversAllNames(t *testing.T) {
	d := store.ConversationDetail{
		ID: "1", ProviderID: "ic_1", Subject: "subj", State: "open", Assignee: "Riley",
		Rating: "5", FinStatus: "resolved", Participants: []string{"a"}, Tags: []string{"t"},
		CreatedAt: "2026-05-18T00:00:00Z", UpdatedAt: "2026-05-19T00:00:00Z", Snippet: "snip",
	}
	for _, name := range []string{"id", "provider_id", "subject", "state", "assignee", "rating", "fin_status", "participants", "tags", "created_at", "updated_at", "snippet", "parts"} {
		if _, err := conversationDetailField(d, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := conversationDetailField(d, "bogus"); err == nil {
		t.Fatalf("expected unknown field error")
	}
}

func TestProjectConversationDetailMaskFiltersFields(t *testing.T) {
	d := store.ConversationDetail{ID: "1", ProviderID: "ic_1", Subject: "subj", Snippet: "snip"}
	got, err := projectConversationDetail(d, "provider_id,subject")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("type = %T", got)
	}
	if m["provider_id"] != "ic_1" || m["subject"] != "subj" {
		t.Fatalf("mask = %#v", m)
	}
	if _, present := m["snippet"]; present {
		t.Fatalf("snippet should not be in projection: %#v", m)
	}
	full, err := projectConversationDetail(d, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := full.(store.ConversationDetail); !ok {
		t.Fatalf("empty mask returns full detail, got %T", full)
	}
	if _, err := projectConversationDetail(d, "subject,bogus"); err == nil {
		t.Fatalf("expected unknown-field error")
	}
}

func TestValidateSearchFieldsAcceptsEmpty(t *testing.T) {
	if err := validateSearchFields("   "); err != nil {
		t.Fatalf("blank: %v", err)
	}
	if err := validateSearchFields("provider_id,score"); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := validateSearchFields("provider_id,bogus"); err == nil {
		t.Fatalf("expected unknown-field error")
	}
}

func TestParseSinceAcceptsRFC3339AndUnix(t *testing.T) {
	now := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	if _, err := parseSince("", now, "updated-since"); err == nil {
		t.Fatalf("empty should error")
	}
	if _, err := parseSince(" -1d ", now, "updated-since"); err == nil {
		t.Fatalf("negative day should error")
	}
	if _, err := parseSince("nope", now, "updated-since"); err == nil {
		t.Fatalf("nonsense should error")
	}
	if _, err := parseSince("-2h", now, "updated-since"); err == nil {
		t.Fatalf("negative duration should error")
	}
	got, err := parseSince("2026-05-15T00:00:00Z", now, "updated-since")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2026 || got.Month() != 5 || got.Day() != 15 {
		t.Fatalf("rfc3339 = %s", got)
	}
	got, err = parseSince("1700000000", now, "updated-since")
	if err != nil {
		t.Fatal(err)
	}
	if got.Unix() != 1700000000 {
		t.Fatalf("unix = %s", got)
	}
}

func TestValidateArchiveArtifactPathRejectsBadShapes(t *testing.T) {
	good := "snapshots/x.jsonl.zst.age"
	if err := validateArchiveArtifactPath("--out", good); err != nil {
		t.Fatalf("good rejected: %v", err)
	}
	bad := []string{
		"",
		"  spaced  ",
		"no-suffix",
		"/abs/path.jsonl.zst.age",
		"has\x01control.jsonl.zst.age",
		"has?query.jsonl.zst.age",
		"has#frag.jsonl.zst.age",
		"escape%2e%2e/x.jsonl.zst.age",
		"../escape.jsonl.zst.age",
		"sub/../escape.jsonl.zst.age",
	}
	for _, b := range bad {
		if err := validateArchiveArtifactPath("--out", b); err == nil {
			t.Fatalf("%q should be rejected", b)
		}
	}
}

func TestHasControlAndHasControlOrSpaceDetect(t *testing.T) {
	if !hasControl("hello\x01world") {
		t.Fatalf("missed control")
	}
	if hasControl("hello world") {
		t.Fatalf("flagged plain text")
	}
	if !hasControlOrSpace("has space") {
		t.Fatalf("missed space")
	}
	if hasControlOrSpace("normal") {
		t.Fatalf("flagged plain")
	}
}

func TestWantsJSONArgParsing(t *testing.T) {
	if !WantsJSON([]string{}) {
		t.Fatalf("default should want JSON")
	}
	if WantsJSON([]string{"--no-json"}) {
		t.Fatalf("--no-json should disable")
	}
	if WantsJSON([]string{"--json=false"}) {
		t.Fatalf("--json=false should disable")
	}
	if !WantsJSON([]string{"--json=true"}) {
		t.Fatalf("--json=true should enable")
	}
	if WantsJSON([]string{"--format=text"}) {
		t.Fatalf("--format=text should disable")
	}
	if !WantsJSON([]string{"--output=json"}) {
		t.Fatalf("--output=json should enable")
	}
	if !WantsJSON([]string{"--json=garbage"}) {
		t.Fatalf("invalid bool should default to wants JSON")
	}
}

func TestSyncCmdModeReportsActiveMode(t *testing.T) {
	cases := []struct {
		cmd  syncCmd
		want string
	}{
		{syncCmd{Fixture: "x"}, "fixture"},
		{syncCmd{UpdatedSince: "1h"}, "updated-since"},
		{syncCmd{Conversation: "c1"}, "conversation"},
		{syncCmd{Entities: true}, "entities"},
		{syncCmd{Resume: true}, "resume"},
		{syncCmd{}, "unknown"},
	}
	for i, tc := range cases {
		if got := tc.cmd.mode(); got != tc.want {
			t.Fatalf("case %d: mode = %q, want %q", i, got, tc.want)
		}
	}
}

func TestShowRetrievesByProviderID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"show", "ic_syn_001", "--fields", "provider_id,subject", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("show: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"provider_id": "ic_syn_001"`)) {
		t.Fatalf("show output: %s", stdout.String())
	}
}

func TestShowRejectsUnknownField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	err := Run(context.Background(), []string{"show", "ic_syn_001", "--fields", "subject,bogus", "--json"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestArchiveDryRunReportsRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/dry.jsonl.zst.age",
		"--dry-run",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"command": "archive"`)) {
		t.Fatalf("archive payload: %s", stdout.String())
	}
}

func TestArchiveRejectsMissingFixture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{
		"archive",
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/dry.jsonl.zst.age",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestPublishDryRunReportsRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	args := []string{
		"publish",
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/publish.jsonl.zst.age",
		"--dry-run",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"command": "publish"`)) {
		t.Fatalf("publish payload: %s", stdout.String())
	}
}

func TestImportRoundTripsEncryptedArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/round.jsonl.zst.age",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("archive: %v", err)
	}
	stdout.Reset()
	args = []string{
		"import",
		"--identity", identity.String(),
		"--in", "tmp/round.jsonl.zst.age",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("import: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"records"`)) {
		t.Fatalf("import payload: %s", stdout.String())
	}
}

func TestImportDryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/in.jsonl.zst.age",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("archive: %v", err)
	}
	stdout.Reset()
	args = []string{
		"import",
		"--identity", identity.String(),
		"--in", "tmp/in.jsonl.zst.age",
		"--dry-run",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("import dry: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"dry_run": true`)) {
		t.Fatalf("import dry payload: %s", stdout.String())
	}
}

func TestVersionTextOutput(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"version", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("version text: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("version text empty")
	}
}

func TestDoctorReadsAgeRecipientPresence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvAgeRecipient, identity.Recipient().String())
	t.Setenv(config.EnvAgeIdentity, identity.String())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor", "--offline", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"`+config.EnvAgeRecipient+`": "present"`) {
		t.Fatalf("recipient not present: %s", out)
	}
	if !strings.Contains(out, `"`+config.EnvIntercomCred+`": "present"`) {
		t.Fatalf("intercom not present: %s", out)
	}
}

var repoRootDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for path := wd; path != "/"; path = filepath.Dir(path) {
		if _, err := os.Stat(filepath.Join(path, "testdata", "synthetic")); err == nil {
			return path
		}
	}
	panic("could not locate repo root from " + wd)
}()

func repoRoot(t *testing.T) string {
	t.Helper()
	return repoRootDir
}

func TestShowWithPartsIncludesPartsList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"show", "ic_syn_001", "--parts", "--part-limit", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("show --parts: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"parts"`)) {
		t.Fatalf("show parts payload: %s", stdout.String())
	}
}

func TestSearchNDJSONStreamsRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--fields", "provider_id,subject", "--ndjson"}, &stdout, &stderr); err != nil {
		t.Fatalf("search ndjson: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("empty ndjson")
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, `{`) {
			t.Fatalf("non-json ndjson line: %q", line)
		}
	}
}

func TestSyncResyncRebuildsState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	args := []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	stdout.Reset()
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"conversations"`)) {
		t.Fatalf("resync payload: %s", stdout.String())
	}
}

func TestDoctorFlagsBadRecipientAndIdentity(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvAgeRecipient, "not-a-recipient")
	t.Setenv(config.EnvAgeIdentity, "not-an-identity")
	t.Setenv(config.EnvIntercomCred, "")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"doctor", "--offline", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected doctor failure")
	}
	out := stdout.String()
	if !strings.Contains(out, `"ok": false`) {
		t.Fatalf("doctor output: %s", out)
	}
}

func TestImportRejectsBadIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/round.jsonl.zst.age",
		"--json",
	}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	err = Run(context.Background(), []string{
		"import",
		"--identity", "not-an-identity",
		"--in", "tmp/round.jsonl.zst.age",
		"--json",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected import with bad identity to fail")
	}
}

func TestPublishRejectsBadRecipient(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"publish",
		"--recipient", "not-a-recipient",
		"--out", "tmp/out.jsonl.zst.age",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected publish with bad recipient to fail")
	}
}

func TestSearchRejectsUnknownField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"search", "x", "--fields", "bogus", "--json"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestSearchNDJSONRejectsUnknownField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"search", "x", "--fields", "bogus", "--ndjson"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestStatusReportsCountsAfterSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"status", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"counts"`)) {
		t.Fatalf("status counts missing: %s", stdout.String())
	}
}

func TestVersionFlagPrintsTextWithoutJSON(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"version", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(stdout.String(), "dev") && !strings.Contains(stdout.String(), "v") {
		t.Fatalf("version text: %s", stdout.String())
	}
}

func TestMetadataNonJSONOutput(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"metadata", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("metadata text: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected text metadata output")
	}
}

func TestDescribeNonJSONOutput(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("describe text: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected text describe output")
	}
}

func TestGuardNonJSONOutputOK(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(wd, "..", ".."))
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"guard", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("guard text: %v", err)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Fatalf("guard text: %s", stdout.String())
	}
}

func TestSyncConversationDryRunDescribesPlan(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--conversation", "ic_conv_42", "--dry-run", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("conversation dry: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"mode": "conversation"`)) {
		t.Fatalf("conversation dry: %s", stdout.String())
	}
}

func TestSyncEntitiesDryRunDescribesPlan(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--entities", "--contacts", "--limit", "10", "--dry-run", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("entities dry: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"mode": "entities"`)) {
		t.Fatalf("entities dry: %s", stdout.String())
	}
}

func TestSyncResumeDryRunDescribesPlan(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--resume", "--dry-run", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("resume dry: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"mode": "resume"`)) {
		t.Fatalf("resume dry: %s", stdout.String())
	}
}

func TestSyncUpdatedSinceRejectsBadDuration(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"sync", "--updated-since", "not-a-duration", "--json"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestArchiveRejectsWeirdRecipient(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", "bogus-recipient",
		"--out", "tmp/x.jsonl.zst.age",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected bad-recipient error")
	}
}

func TestArchiveRejectsBadOutPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "no-suffix",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestPublishRejectsBadOutPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{
		"publish",
		"--recipient", identity.Recipient().String(),
		"--out", "no-suffix",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestImportRejectsBadInPath(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"import",
		"--identity", "AGE-SECRET-KEY-1JUNK",
		"--in", "no-suffix",
		"--json",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestPublishWritesEncryptedArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{
		"publish",
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/published.jsonl.zst.age",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"records"`)) {
		t.Fatalf("publish output: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, "tmp", "published.jsonl.zst.age")); err != nil {
		t.Fatalf("published file: %v", err)
	}
}

func TestArchiveWritesEncryptedArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "tmp/archived.jsonl.zst.age",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "tmp", "archived.jsonl.zst.age")); err != nil {
		t.Fatalf("archive file: %v", err)
	}
}

func TestPublishUsesRecipientFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvAgeRecipient, identity.Recipient().String())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{
		"publish",
		"--out", "tmp/env-published.jsonl.zst.age",
		"--dry-run",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("publish env-recipient: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"command": "publish"`)) {
		t.Fatalf("publish env-recipient: %s", stdout.String())
	}
}

func TestArchiveUsesRecipientFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvAgeRecipient, identity.Recipient().String())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--out", "tmp/env.jsonl.zst.age",
		"--dry-run",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("archive env-recipient: %v", err)
	}
}

func TestCommandsErrorWhenFINCRAWLHomeIsAFile(t *testing.T) {
	// Pointing FINCRAWL_HOME at a regular file causes config.LoadRuntime to fail,
	// which exercises the early error returns in every command's Run().
	home := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(home, []byte("file-not-dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINCRAWL_HOME", home)
	for _, cmd := range [][]string{
		{"doctor", "--offline", "--json"},
		{"metadata", "--json"},
		{"status", "--json"},
		{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--dry-run", "--json"},
		{"archive", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--recipient", "age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl", "--out", "tmp/x.jsonl.zst.age", "--dry-run", "--json"},
		{"publish", "--recipient", "age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl", "--out", "tmp/x.jsonl.zst.age", "--dry-run", "--json"},
		{"import", "--identity", "AGE-SECRET-KEY-1JUNK", "--in", "tmp/x.jsonl.zst.age", "--json"},
		{"subscribe", t.TempDir(), "--dry-run", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		if err := Run(context.Background(), cmd, &stdout, &stderr); err == nil {
			t.Fatalf("%v: expected error", cmd)
		}
	}
}

func TestSearchHonoursStateAndTagFilters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"), "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--state", "open", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("search state: %v", err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "Morgan", "--tag", "billing", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("search tag: %v", err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"search", "billing", "--fin-status", "resolved", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("search fin-status: %v", err)
	}
}

func TestSubscribeWithBadStoreFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	// store dir without manifest -> Verify fails
	bad := filepath.Join(home, "bad-store")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"subscribe", bad, "--dry-run", "--json"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected subscribe to fail with bad store")
	}
}

func TestStoreVerifyJSONOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	bad := filepath.Join(home, "bad-store")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"store", "verify", bad, "--json"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected store verify to fail without manifest")
	}
}

func TestSubscribeDryRunListsSnapshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	// build a valid encrypted tenant store
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "snapshots/one.jsonl.zst.age",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "manifest.json"),
		[]byte(`{"version":"1","snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{
		"subscribe", home,
		"--identity", identity.String(),
		"--dry-run",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("subscribe dry: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"dry_run": true`)) {
		t.Fatalf("subscribe dry payload: %s", stdout.String())
	}
}

func TestSubscribeImportsSnapshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	t.Chdir(home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "snapshots/one.jsonl.zst.age",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "manifest.json"),
		[]byte(`{"version":"1","snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	// Use FINCRAWL_HOME for archive db inside the tenant store dir
	freshHome := t.TempDir()
	t.Setenv("FINCRAWL_HOME", freshHome)
	if err := Run(context.Background(), []string{
		"subscribe", home,
		"--identity", identity.String(),
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"imported_snapshots"`)) {
		t.Fatalf("subscribe payload: %s", stdout.String())
	}
}

func TestStoreVerifyJSONReportsValidStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	tenant := filepath.Join(home, "tenant")
	if err := os.MkdirAll(filepath.Join(tenant, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot(t), "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", "tenant/snapshots/one.jsonl.zst.age",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tenant, "manifest.json"),
		[]byte(`{"version":"1","snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"store", "verify", tenant, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("store verify: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok": true`)) {
		t.Fatalf("store verify payload: %s", stdout.String())
	}
}

func TestDoctorTextModeOKPrintsOK(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvAgeRecipient, "")
	t.Setenv(config.EnvAgeIdentity, "")
	t.Setenv(config.EnvIntercomCred, "")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor", "--offline", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("doctor text: %v", err)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Fatalf("doctor text: %s", stdout.String())
	}
}

func TestDoctorTextModeFailReturnsError(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvAgeRecipient, "not-a-recipient")
	t.Setenv(config.EnvAgeIdentity, "")
	t.Setenv(config.EnvIntercomCred, "")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor", "--offline", "--json=false"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected doctor failure")
	}
}

func TestGuardFailingExitsNonZero(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Chdir(tmp)
	// Initialize a git repo with a forbidden file to trigger a finding.
	out, err := exec.Command("git", "init", "--quiet").CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "config", "user.email", "test@example.com").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "config", "user.name", "Test").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("X=y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"guard", "--json"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected guard failure with finding")
	}
}
