package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/uinaf/fincrawl/internal/config"
)

func writeArchiveDB(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FINCRAWL_HOME", home)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join("..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatalf("seed sync: %v\n%s", err, stderr.String())
	}
	return home
}

func TestPublishDryRunReportsRecordCount(t *testing.T) {
	writeArchiveDB(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out := "out.jsonl.zst.age"
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"publish", "--recipient", identity.Recipient().String(), "--out", out, "--dry-run", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("publish dry-run: %v\nstderr=%s", err, stderr.String())
	}
	var plan map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan["records"] == nil {
		t.Fatalf("plan missing records: %#v", plan)
	}
}

func TestPublishRejectsBadRecipientCoverage(t *testing.T) {
	writeArchiveDB(t)
	t.Chdir(t.TempDir())
	out := "out.jsonl.zst.age"
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"publish", "--recipient", "not-a-key", "--out", out}, &stdout, &stderr); err == nil {
		t.Fatalf("expected publish to reject bad recipient")
	}
}

func TestPublishCreatesNestedOutputDirectory(t *testing.T) {
	writeArchiveDB(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out := filepath.Join("nested", "deep", "out.jsonl.zst.age")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"publish", "--recipient", identity.Recipient().String(), "--out", out, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("publish: %v\nstderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
}

func TestArchiveDryRunReportsRecordCount(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out := "archive.jsonl.zst.age"
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot, "..", "..", "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", out,
		"--dry-run", "--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("archive dry-run: %v\nstderr=%s", err, stderr.String())
	}
}

func TestArchiveWritesEncryptedSnapshot(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out := "archive.jsonl.zst.age"
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--fixture", filepath.Join(repoRoot, "..", "..", "testdata", "synthetic"),
		"--recipient", identity.Recipient().String(),
		"--out", out,
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("archive: %v\nstderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected archive file: %v", err)
	}
}

func TestArchiveRequiresFixture(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out := "archive.jsonl.zst.age"
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"archive",
		"--recipient", identity.Recipient().String(),
		"--out", out,
	}, &stdout, &stderr); err == nil {
		t.Fatalf("expected error when --fixture missing")
	}
}

func TestSearchNDJSONEmitsOneRecordPerLine(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"search", "billing", "--ndjson", "--fields", "provider_id,subject"}, &stdout, &stderr); err != nil {
		t.Fatalf("search ndjson: %v\nstderr=%s", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("ndjson line is not JSON object: %v\n%s", err, line)
		}
	}
}

func TestSearchNDJSONWithoutFieldsEmitsFullRows(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"search", "billing", "--ndjson"}, &stdout, &stderr); err != nil {
		t.Fatalf("search ndjson all: %v\nstderr=%s", err, stderr.String())
	}
}

func TestShowConversationWithParts(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"search", "billing", "--fields", "provider_id"}, &stdout, &stderr); err != nil {
		t.Fatalf("search: %v\nstderr=%s", err, stderr.String())
	}
	var results []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("decode search: %v\n%s", err, stdout.String())
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one search result")
	}
	providerID, _ := results[0]["provider_id"].(string)
	if providerID == "" {
		t.Fatalf("expected provider_id in search result: %#v", results[0])
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"show", providerID, "--parts", "--part-limit", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("show: %v\nstderr=%s", err, stderr.String())
	}
}

func TestShowConversationWithFields(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"search", "billing", "--fields", "provider_id"}, &stdout, &stderr); err != nil {
		t.Fatalf("search: %v\nstderr=%s", err, stderr.String())
	}
	var results []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("decode search: %v\n%s", err, stdout.String())
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one search result")
	}
	providerID, _ := results[0]["provider_id"].(string)
	stdout.Reset()
	if err := Run(context.Background(), []string{"show", providerID, "--fields", "provider_id,subject", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("show fields: %v\nstderr=%s", err, stderr.String())
	}
}

func TestShowRejectsBadFields(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"show", "ic_anything", "--fields", "not_a_field"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected show to reject bad fields")
	}
}

func TestSyncConversationRejectsInvalidProviderID(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Setenv(config.EnvIntercomCred, "fake-token")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--conversation", "../etc/passwd"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected sync to reject bad conversation id")
	}
}

func TestSubscribeRejectsNonJSONLZstAgeSnapshot(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	storeRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(storeRoot, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/foo.bin"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(storeRoot, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "snapshots", "foo.bin"), []byte("not encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"subscribe", storeRoot, "--dry-run", "--json"}, &stdout, &stderr); err == nil {
		t.Fatalf("expected subscribe to reject non-encrypted snapshot")
	}
}

func TestSubscribeIdentityFromEnv(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceHome := t.TempDir()
	t.Setenv("FINCRAWL_HOME", sourceHome)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	t.Chdir(work)
	storeRoot := "store"
	out := filepath.Join(storeRoot, "snapshots", "synthetic.jsonl.zst.age")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--fixture", filepath.Join(repoRoot, "..", "..", "testdata", "synthetic")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"publish", "--recipient", identity.Recipient().String(), "--out", out}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/synthetic.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	targetHome := t.TempDir()
	t.Setenv("FINCRAWL_HOME", targetHome)
	t.Setenv(config.EnvAgeIdentity, identity.String())
	stdout.Reset()
	if err := Run(context.Background(), []string{"subscribe", storeRoot, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("subscribe (identity from env): %v\nstderr=%s", err, stderr.String())
	}
}

func TestStatusCommandReadsCounts(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"status", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("status: %v\nstderr=%s", err, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"counts"`)) {
		t.Fatalf("status missing counts: %s", stdout.String())
	}
}

func TestStatusCommandTextMode(t *testing.T) {
	writeArchiveDB(t)
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"status", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("status text: %v\nstderr=%s", err, stderr.String())
	}
}

func TestStatusCommandFreshHome(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"status", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("status fresh: %v\nstderr=%s", err, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"empty"`)) {
		t.Fatalf("expected empty state: %s", stdout.String())
	}
}

func TestDescribeAllCommands(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("describe: %v\nstderr=%s", err, stderr.String())
	}
}

func TestDescribeSpecificCommand(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "sync", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("describe sync: %v\nstderr=%s", err, stderr.String())
	}
}

func TestDescribeTextMode(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("describe text: %v\nstderr=%s", err, stderr.String())
	}
}

func TestMetadataJSONIncludesAppID(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"metadata", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("metadata: %v\nstderr=%s", err, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("fincrawl")) {
		t.Fatalf("metadata missing app id: %s", stdout.String())
	}
}

func TestMetadataTextMode(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"metadata", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("metadata text: %v\nstderr=%s", err, stderr.String())
	}
}

func TestVersionTextMode(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"version", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("version text: %v\nstderr=%s", err, stderr.String())
	}
}

func TestStoreVerifyHappyPath(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	storeRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(storeRoot, "manifest.json"), []byte(`{"snapshots":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"store", "verify", storeRoot, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("store verify: %v\nstderr=%s", err, stderr.String())
	}
}

func TestGuardHappyPathOnEmptyGitRepo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Chdir(tmp)
	if out, err := exec.Command("git", "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "config", "user.email", "test@example.com").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "config", "user.name", "Test").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "add", "README.md").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "commit", "-m", "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"guard", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("guard: %v\nstderr=%s", err, stderr.String())
	}
}

func TestGuardTextMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	t.Chdir(tmp)
	if out, err := exec.Command("git", "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "config", "user.email", "test@example.com").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "config", "user.name", "Test").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"guard", "--json=false"}, &stdout, &stderr); err != nil {
		t.Fatalf("guard text: %v\nstderr=%s", err, stderr.String())
	}
}
