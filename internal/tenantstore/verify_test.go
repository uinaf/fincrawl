package tenantstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyAcceptsEncryptedSnapshots(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"version":"1","snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Snapshots != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestVerifyAcceptsCompactSnapshotManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":["snapshots/one.jsonl.zst.age"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Snapshots != 1 || len(report.Findings) != 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestVerifyRejectsTraversalAndPlaintext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "plain.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":["../outside.jsonl.zst.age","snapshots/plain.jsonl"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if len(report.Findings) < 3 {
		t.Fatalf("findings = %#v, want traversal, encrypted suffix, and plaintext findings", report.Findings)
	}
}

func TestVerifyRejectsSymlinkedSnapshot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jsonl.zst.age")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "snapshots", "one.jsonl.zst.age")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "symlink") {
		t.Fatalf("findings = %#v, want symlink finding", report.Findings)
	}
}

func TestVerifyRejectsSymlinkedSnapshotParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "one.jsonl.zst.age"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "snapshots")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "symlink") {
		t.Fatalf("findings = %#v, want symlink finding", report.Findings)
	}
}

func TestVerifyRejectsSymlinkedRoot(t *testing.T) {
	realRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(realRoot, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "debug.log"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "store-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "root must not be a symlink") {
		t.Fatalf("findings = %#v, want root symlink finding", report.Findings)
	}
}

func TestVerifyRejectsSymlinkedManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(outside, []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "manifest.json")); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "manifest must not be a symlink") {
		t.Fatalf("findings = %#v, want manifest symlink finding", report.Findings)
	}
}

func TestVerifyAcceptsNonArtifactDocSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("agent guide"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("AGENTS.md", filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("report = %#v", report)
	}
}

func TestVerifyRejectsNonDocSymlinkEvenWithSafeLookingName(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "archive.db"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "archive.db"), filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "must not contain symlinks") {
		t.Fatalf("findings = %#v, want symlink finding", report.Findings)
	}
}

func TestVerifySkipsGitIgnoredLocalRuntimeDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	root := t.TempDir()
	if err := runGit(root, "init"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("state/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "archive.db"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("report = %#v", report)
	}
}

func TestVerifyDoesNotUseParentGitIgnoreForNestedStore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	parent := t.TempDir()
	if err := runGit(parent, "init"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte("tmp/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "tmp", "store")
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "archive.db"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "runtime or report directories") {
		t.Fatalf("findings = %#v, want runtime directory finding", report.Findings)
	}
}

func TestVerifyRejectsRuntimeReportFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"debug.log", "daily-report.json", "support-screenshot.png", "case-transcript.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "runtime or report artifacts") {
		t.Fatalf("findings = %#v, want runtime/report findings", report.Findings)
	}
}

func TestVerifyRejectsSymlinkedRuntimeReportDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "case-transcript.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "transcripts")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "symlinks") {
		t.Fatalf("findings = %#v, want symlink finding", report.Findings)
	}
}

func TestVerifyRejectsSQLiteSidecars(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"archive.db-wal", "archive.db-shm", "archive.sqlite-wal", "archive.sqlite3-shm"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := Verify(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "plaintext archive artifacts") {
		t.Fatalf("findings = %#v, want SQLite sidecar findings", report.Findings)
	}
}

func TestVerifyReportsIncompleteScanWhenContextCanceled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := Verify(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("report unexpectedly OK: %#v", report)
	}
	if !hasFindingReason(report, "scan incomplete") {
		t.Fatalf("findings = %#v, want incomplete scan finding", report.Findings)
	}
}

func hasFindingReason(report Report, text string) bool {
	for _, finding := range report.Findings {
		if strings.Contains(finding.Reason, text) {
			return true
		}
	}
	return false
}

func runGit(root string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.Run()
}

func TestVerifiedSnapshotsReturnsFilesOnSuccess(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"version":"1","snapshots":[{"path":"snapshots/one.jsonl.zst.age","records":42,"created_at":"2026-05-19T00:00:00Z"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report, files, err := VerifiedSnapshots(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || len(files) != 1 {
		t.Fatalf("report = %#v files = %#v", report, files)
	}
	f := files[0]
	if f.Path != filepath.Join("snapshots", "one.jsonl.zst.age") {
		t.Fatalf("path = %q", f.Path)
	}
	if f.Records != 42 || f.CreatedAt != "2026-05-19T00:00:00Z" {
		t.Fatalf("file = %#v", f)
	}
	if !strings.HasPrefix(f.FullPath, root) {
		t.Fatalf("full path %q not under root %q", f.FullPath, root)
	}
}

func TestVerifiedSnapshotsReturnsNilOnFailedVerify(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "snapshots", "one.jsonl.zst.age"), []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"version":"1","snapshots":[{"path":"snapshots/one.jsonl.zst.age"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "leak.db"), []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, files, err := VerifiedSnapshots(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("expected verify to fail on plaintext leak, got %#v", report)
	}
	if files != nil {
		t.Fatalf("expected nil files on failure, got %#v", files)
	}
}

func TestPlaintextArtifactClassifies(t *testing.T) {
	for path, want := range map[string]bool{
		"snapshots/x.jsonl":         true,
		"snapshots/x.jsonl.zst":     true,
		"data/store.db":             true,
		"data/store.sqlite-wal":     true,
		"data/store.tar":            true,
		"data/store.tar.zst":        true,
		"snapshots/x.jsonl.zst.age": false,
		"snapshots/x.tar.zst.age":   false,
		"docs/architecture.md":      false,
	} {
		if got := plaintextArtifact(path); got != want {
			t.Fatalf("plaintextArtifact(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestPlaintextRuntimeFileClassifies(t *testing.T) {
	for path, want := range map[string]bool{
		"logs/run.log":              true,
		"trace.har":                 true,
		"report-summary.json":       true,
		"screenshot-home.png":       true,
		"screenshot-overview.pdf":   true,
		"transcript-call.md":        true,
		"transcript-call.json":      true,
		"docs/architecture.md":      false,
		"snapshots/x.jsonl.zst.age": false,
	} {
		if got := plaintextRuntimeFile(path); got != want {
			t.Fatalf("plaintextRuntimeFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRuntimeDirectoryNameRecognizes(t *testing.T) {
	for name, want := range map[string]bool{
		"logs": true, "reports": true, "screenshots": true,
		"state": true, "transcripts": true,
		"docs": false, "snapshots": false,
	} {
		if got := runtimeDirectoryName(name); got != want {
			t.Fatalf("runtimeDirectoryName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestEncryptedSnapshotPathRecognizes(t *testing.T) {
	for path, want := range map[string]bool{
		"snapshots/x.jsonl.zst.age": true,
		"snapshots/x.tar.zst.age":   true,
		"snapshots/x.jsonl":         false,
	} {
		if got := encryptedSnapshotPath(path); got != want {
			t.Fatalf("encryptedSnapshotPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestHasUnsafePathTextRejectsControlAndPctEncoded(t *testing.T) {
	for path, want := range map[string]bool{
		"normal/path":     false,
		"has space":       false,
		"has\x01control":  true,
		"with?query":      true,
		"with#frag":       true,
		"pct%2eencoded":   true,
		"pct%2Fseparator": true,
		"pct%5Cbackslash": true,
	} {
		if got := hasUnsafePathText(path); got != want {
			t.Fatalf("hasUnsafePathText(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestAllowedDocSymlinkOnlyForClaudeMdToAgents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink("AGENTS.md", link); err != nil {
		t.Fatal(err)
	}
	if !allowedDocSymlink(link, "CLAUDE.md") {
		t.Fatalf("expected allowed symlink")
	}
	bad := filepath.Join(dir, "OTHER.md")
	if err := os.Symlink("AGENTS.md", bad); err != nil {
		t.Fatal(err)
	}
	if allowedDocSymlink(bad, "OTHER.md") {
		t.Fatalf("only CLAUDE.md should be allowed")
	}
}
