package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/openclaw/crawlkit/output"
	"github.com/uinaf/fincrawl/internal/config"
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
