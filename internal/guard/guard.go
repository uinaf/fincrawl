package guard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Finding struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Result struct {
	OK       bool      `json:"ok"`
	Findings []Finding `json:"findings,omitempty"`
	Scanned  int       `json:"scanned"`
}

const (
	opScheme                  = "op" + "://"
	maxContentScanBytes int64 = 2 * 1024 * 1024
)

var (
	secretPattern                  = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]{20,}|(api|access|intercom)[_-]?(token|key|secret)\s*[:=]\s*["']?[a-z0-9._~+/=-]{16,})`)
	opRefPattern                   = regexp.MustCompile(regexp.QuoteMeta(opScheme) + `[^\s"')` + "`" + `]+`)
	intercomConversationURLPattern = regexp.MustCompile(`(?i)https?://app\.intercom\.com/[^\s"')` + "`" + `]*(conversation|inbox)[^\s"')` + "`" + `]*`)
	providerIdentifierPattern      = regexp.MustCompile(`(?i)"(workspace_id|app_id|account_id|conversation_id|contact_id|admin_id|team_id)"\s*:\s*"[^"<][^"]{2,}"`)
)

func Run(root string) (Result, error) {
	repoRoot, err := gitRoot(root)
	if err != nil {
		return Result{}, err
	}
	paths, err := gitCandidates(repoRoot)
	if err != nil {
		return Result{}, err
	}
	result, err := CheckFiles(repoRoot, paths)
	if err != nil {
		return Result{}, err
	}
	cachedPaths, err := gitCachedPaths(repoRoot)
	if err != nil {
		return Result{}, err
	}
	for _, path := range cachedPaths {
		clean := cleanPath(path)
		if clean == "" {
			continue
		}
		if reason := forbiddenPath(clean); reason != "" {
			result.addFinding(clean, reason)
			continue
		}
		if err := scanGitBlob(repoRoot, clean, &result); err != nil {
			return Result{}, err
		}
	}
	result.finish()
	return result, nil
}

func CheckFiles(root string, paths []string) (Result, error) {
	var result Result
	for _, path := range paths {
		clean := cleanPath(path)
		if clean == "" {
			continue
		}
		result.Scanned++
		if reason := forbiddenPath(clean); reason != "" {
			result.addFinding(clean, reason)
			continue
		}
		if err := scanWorkingTreeFile(root, clean, &result); err != nil {
			return Result{}, err
		}
	}
	result.finish()
	return result, nil
}

func scanWorkingTreeFile(root, clean string, result *Result) error {
	fullPath := filepath.Join(root, clean)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		return nil
	}
	if info.Size() > maxContentScanBytes {
		scanLargeCandidate(clean, result)
		return nil
	}
	body, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", clean, err)
	}
	for _, reason := range forbiddenContent(clean, body) {
		result.addFinding(clean, reason)
	}
	return nil
}

func scanGitBlob(root, clean string, result *Result) error {
	// #nosec G204 -- clean is a guard-filtered relative path; argv form, no shell.
	sizeCmd := exec.Command("git", "cat-file", "-s", ":"+clean)
	sizeCmd.Dir = root
	sizeOut, err := sizeCmd.Output()
	if err != nil {
		return fmt.Errorf("read staged size %s: %w", clean, err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64)
	if err != nil {
		return fmt.Errorf("parse staged size %s: %w", clean, err)
	}
	if size > maxContentScanBytes {
		scanLargeCandidate(clean, result)
		return nil
	}
	// #nosec G204 -- clean is a guard-filtered relative path; argv form, no shell.
	bodyCmd := exec.Command("git", "cat-file", "-p", ":"+clean)
	bodyCmd.Dir = root
	body, err := bodyCmd.Output()
	if err != nil {
		return fmt.Errorf("read staged blob %s: %w", clean, err)
	}
	for _, reason := range forbiddenContent(clean, body) {
		result.addFinding(clean, reason)
	}
	return nil
}

func scanLargeCandidate(path string, result *Result) {
	if !isSyntheticFixturePath(path) {
		result.addFinding(path, "large file requires manual guard review")
	}
}

func (result *Result) addFinding(path, reason string) {
	for _, finding := range result.Findings {
		if finding.Path == path && finding.Reason == reason {
			return
		}
	}
	result.Findings = append(result.Findings, Finding{Path: path, Reason: reason})
}

func (result *Result) finish() {
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].Path == result.Findings[j].Path {
			return result.Findings[i].Reason < result.Findings[j].Reason
		}
		return result.Findings[i].Path < result.Findings[j].Path
	})
	result.OK = len(result.Findings) == 0
}

func gitCandidates(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list git candidates: %w", err)
	}
	return splitNullPaths(out), nil
}

func gitCachedPaths(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list git cached files: %w", err)
	}
	return splitNullPaths(out), nil
}

func splitNullPaths(out []byte) []string {
	parts := bytes.Split(out, []byte{0})
	var paths []string
	for _, part := range parts {
		if len(part) > 0 {
			paths = append(paths, string(part))
		}
	}
	sort.Strings(paths)
	return paths
}

func cleanPath(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || strings.HasPrefix(clean, ".git/") {
		return ""
	}
	return clean
}

func gitRoot(start string) (string, error) {
	// #nosec G204 -- start is the caller-supplied scan root; argv form, no shell.
	cmd := exec.Command("git", "-C", start, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve git root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func forbiddenPath(path string) string {
	base := filepath.Base(path)
	switch base {
	case ".env", ".env.local":
		return "local env files must not be committed"
	case ".env.example", ".env.local.example":
		return ""
	}
	if strings.HasPrefix(base, ".env.") {
		return "local env files must not be committed"
	}
	if isGeneratedArtifactPath(path) {
		return "generated tenant artifact path"
	}
	switch {
	case strings.HasSuffix(path, ".jsonl.zst.age"), strings.HasSuffix(path, ".tar.zst.age"):
		return "generated encrypted archive artifact"
	case strings.HasSuffix(path, ".jsonl"):
		return "plaintext JSONL archive artifact"
	case strings.HasSuffix(path, ".jsonl.gz"), strings.HasSuffix(path, ".jsonl.zst"):
		return "compressed plaintext archive artifact"
	case strings.HasSuffix(path, ".tar"), strings.HasSuffix(path, ".tar.gz"), strings.HasSuffix(path, ".tar.zst"):
		return "plaintext archive artifact"
	case strings.HasSuffix(path, ".sqlite"), strings.HasSuffix(path, ".sqlite3"), strings.HasSuffix(path, ".db"),
		strings.HasSuffix(path, ".sqlite-wal"), strings.HasSuffix(path, ".sqlite-shm"), strings.HasSuffix(path, ".sqlite-journal"),
		strings.HasSuffix(path, ".sqlite3-wal"), strings.HasSuffix(path, ".sqlite3-shm"), strings.HasSuffix(path, ".sqlite3-journal"),
		strings.HasSuffix(path, ".db-wal"), strings.HasSuffix(path, ".db-shm"), strings.HasSuffix(path, ".db-journal"):
		return "local SQLite archive artifact"
	case strings.HasSuffix(path, ".log"):
		return "local log artifact"
	case strings.HasSuffix(path, ".har"):
		return "browser/network capture artifact"
	}
	return ""
}

func isGeneratedArtifactPath(path string) bool {
	for _, part := range strings.Split(path, "/") {
		switch part {
		case "snapshots", "reports", "screenshots", "logs", "transcripts":
			return true
		}
	}
	return false
}

func forbiddenContent(path string, body []byte) []string {
	text := string(body)
	var reasons []string
	if containsProviderArtifact(text) && !isSyntheticFixturePath(path) {
		reasons = append(reasons, "provider-specific artifact")
	}
	if containsRealOPReference(text) {
		reasons = append(reasons, "real 1Password reference")
	}
	if secretPattern.MatchString(text) {
		reasons = append(reasons, "secret-looking value")
	}
	if shouldScanTranscriptContent(path) && !isSyntheticFixturePath(path) && looksTranscriptLike(text) {
		reasons = append(reasons, "transcript-like data outside synthetic fixtures")
	}
	return reasons
}

func containsRealOPReference(text string) bool {
	for _, ref := range opRefPattern.FindAllString(text, -1) {
		if !strings.HasPrefix(ref, opScheme+"<") {
			return true
		}
	}
	return false
}

func containsProviderArtifact(text string) bool {
	return intercomConversationURLPattern.MatchString(text) || providerIdentifierPattern.MatchString(text)
}

func isSyntheticFixturePath(path string) bool {
	return strings.Contains(path, "testdata/synthetic/")
}

func shouldScanTranscriptContent(path string) bool {
	switch filepath.Ext(path) {
	case ".json", ".jsonl", ".txt", ".log", ".csv", ".tsv":
		return true
	default:
		return false
	}
}

func looksTranscriptLike(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "conversation_parts") ||
		(strings.Contains(lower, `"body"`) && strings.Contains(lower, `"author`)) ||
		(strings.Contains(lower, "transcript") && strings.Contains(lower, "conversation"))
}
