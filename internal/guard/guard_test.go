package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFilesBlocksLocalArtifactsAndSecrets(t *testing.T) {
	root := t.TempDir()
	write(t, root, "leak.jsonl", "{}\n")
	write(t, root, "encrypted.jsonl.zst.age", "AGE-ENCRYPTED-FILE")
	write(t, root, "archive.db-wal", "sqlite sidecar")
	write(t, root, "notes.txt", "Authorization: Bearer "+"abcdefghijklmnopqrstuvwxyz123456\n")
	write(t, root, "provider.json", `{"workspace_`+`id":"real_workspace_123"}`)
	result, err := CheckFiles(root, []string{"leak.jsonl", "encrypted.jsonl.zst.age", "archive.db-wal", "notes.txt", "provider.json"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard unexpectedly passed")
	}
	if len(result.Findings) != 5 {
		t.Fatalf("findings = %d, want 5: %#v", len(result.Findings), result.Findings)
	}
}

func TestCheckFilesBlocksProviderConversationURL(t *testing.T) {
	root := t.TempDir()
	write(t, root, "notes.md", "See https://app."+"intercom.com/a/inbox/example/inbox/conversation/123\n")
	result, err := CheckFiles(root, []string{"notes.md"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard allowed provider conversation URL")
	}
}

func TestCheckFilesBlocksGeneratedTenantArtifactPaths(t *testing.T) {
	root := t.TempDir()
	write(t, root, "reports/live-smoke.txt", "redacted\n")
	write(t, root, "debug/session.har", "{}")
	result, err := CheckFiles(root, []string{"reports/live-smoke.txt", "debug/session.har"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard allowed generated tenant artifacts")
	}
	if len(result.Findings) != 2 {
		t.Fatalf("findings = %#v", result.Findings)
	}
}

func TestCheckFilesScansGoSourceForSecrets(t *testing.T) {
	root := t.TempDir()
	write(t, root, "leak_test.go", `package main

const token = "`+"intercom_"+"token = "+"abcdefghijklmnopqrstuvwxyz123456"+`"
`)
	result, err := CheckFiles(root, []string{"leak_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard allowed secret-looking Go source")
	}
}

func TestCheckFilesAllowsSyntheticFixture(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("testdata", "synthetic", "conversations.json")
	write(t, root, path, `{"conversation_parts":[{"body":"fake","author_name":"Example"}]}`)
	result, err := CheckFiles(root, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("guard blocked synthetic fixture: %#v", result.Findings)
	}
}

func TestCheckFilesBlocksMixedRealOPReference(t *testing.T) {
	root := t.TempDir()
	write(t, root, "template.env", `A="{{ `+opScheme+`<vault>/<item>/<field> }}"
B="{{ `+opScheme+`Private/RealItem/password }}"`)
	result, err := CheckFiles(root, []string{"template.env"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard allowed mixed real op reference")
	}
}

func TestRunUsesGitRoot(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	write(t, root, "leak.jsonl", "{}\n")
	subdir := filepath.Join(root, "internal")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Run(subdir)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard missed root leak from subdir")
	}
}

func TestRunScansStagedBlob(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	write(t, root, "leak.txt", "Authorization: Bearer "+"abcdefghijklmnopqrstuvwxyz123456\n")
	runGit(t, root, "add", "leak.txt")
	write(t, root, "leak.txt", "clean\n")
	result, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard missed staged secret")
	}
}

func TestCheckFilesFailsClosedOnLargeNonSyntheticCandidate(t *testing.T) {
	root := t.TempDir()
	write(t, root, "notes.go", `package main
const blob = "`+strings.Repeat("x", int(maxContentScanBytes)+1)+`"
`)
	result, err := CheckFiles(root, []string{"notes.go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("guard allowed large non-synthetic candidate")
	}
}

func write(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestForbiddenPathClassifiesArtifactTypes(t *testing.T) {
	cases := map[string]bool{
		".env":                   true,
		".env.local":             true,
		".env.something":         true,
		".env.example":           false,
		".env.local.example":     false,
		"snapshots/foo.txt":      true,
		"reports/page.html":      true,
		"screenshots/x.png":      true,
		"logs/run.log":           true,
		"transcripts/x.txt":      true,
		"data/archive.jsonl":     true,
		"data/archive.jsonl.gz":  true,
		"data/archive.tar":       true,
		"data/archive.tar.gz":    true,
		"data/store.sqlite":      true,
		"data/store.db":          true,
		"data/store.sqlite-wal":  true,
		"trace.har":              true,
		"x.jsonl.zst.age":        true,
		"docs/architecture.md":   false,
		"internal/cli/cli.go":    false,
	}
	for path, want := range cases {
		got := forbiddenPath(path)
		if (got != "") != want {
			t.Fatalf("forbiddenPath(%q) = %q, want forbidden=%v", path, got, want)
		}
	}
}

func TestRunReportsScannedZeroForEmptyDir(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "commit", "--allow-empty", "-m", "init", "--quiet")
	result, err := Run(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("empty repo flagged: %#v", result.Findings)
	}
}

func TestRunFlagsLogsAndProviderURLsInRepo(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	// Commit a forbidden plaintext archive path so it ends up in the working tree:
	write(t, repo, "logs/run.log", "ok\n")
	write(t, repo, "snapshots/x.jsonl", "plain\n")
	// .env files
	write(t, repo, ".env", "SECRET=x\n")
	result, err := Run(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("expected findings: %#v", result)
	}
	if len(result.Findings) < 3 {
		t.Fatalf("findings = %d, want >= 3: %#v", len(result.Findings), result.Findings)
	}
}

func TestRunDetectsSecretLookingContent(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	// Compose at runtime so guard scanning *this* test file does not flag it.
	secret := "INTERCOM_" + "TOKEN" + "=" + "abcdef0123456789ABCD"
	write(t, repo, "config.txt", secret+"\n")
	result, err := Run(repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("expected secret finding: %#v", result.Findings)
	}
}

func TestForbiddenPathPermitsExampleEnv(t *testing.T) {
	if forbiddenPath(".env.example") != "" {
		t.Fatalf("example env should not be forbidden")
	}
	if forbiddenPath(".env.local.example") != "" {
		t.Fatalf("example local env should not be forbidden")
	}
	if forbiddenPath(".env.local") == "" {
		t.Fatalf("local env should be forbidden")
	}
}
