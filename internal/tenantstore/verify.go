package tenantstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

type Manifest struct {
	Version   string     `json:"version,omitempty"`
	Snapshots []Snapshot `json:"snapshots"`
}

type Snapshot struct {
	Path      string `json:"path"`
	Kind      string `json:"kind,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Records   int    `json:"records,omitempty"`
}

type SnapshotFile struct {
	Path      string `json:"path"`
	Kind      string `json:"kind,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Records   int    `json:"records,omitempty"`
	FullPath  string `json:"-"`
}

type Report struct {
	OK        bool      `json:"ok"`
	Root      string    `json:"root"`
	Manifest  string    `json:"manifest"`
	Snapshots int       `json:"snapshots"`
	Findings  []Finding `json:"findings,omitempty"`
}

type Finding struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func Verify(ctx context.Context, root string) (Report, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	report := Report{OK: true, Root: absRoot, Manifest: filepath.Join(absRoot, "manifest.json")}
	rootInfo, err := os.Lstat(absRoot)
	if err != nil {
		return Report{}, fmt.Errorf("inspect tenant store root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		report.add(Finding{Path: ".", Reason: "tenant store root must not be a symlink"})
		report.OK = false
		return report, nil
	}
	if !rootInfo.IsDir() {
		report.add(Finding{Path: ".", Reason: "tenant store root must be a directory"})
		report.OK = false
		return report, nil
	}
	manifestInfo, err := os.Lstat(report.Manifest)
	if err != nil {
		return Report{}, fmt.Errorf("inspect tenant store manifest: %w", err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 {
		report.add(Finding{Path: "manifest.json", Reason: "tenant store manifest must not be a symlink"})
		report.OK = false
		return report, nil
	}
	manifest, err := readManifest(report.Manifest)
	if err != nil {
		return Report{}, err
	}
	for _, snapshot := range manifest.Snapshots {
		report.Snapshots++
		report.add(validateSnapshotPath(absRoot, snapshot.Path)...)
	}
	findings, err := scanStoreRoot(ctx, absRoot)
	report.add(findings...)
	if err != nil {
		report.add(Finding{Path: ".", Reason: "tenant store scan incomplete: " + err.Error()})
	}
	report.OK = len(report.Findings) == 0
	return report, nil
}

func VerifiedSnapshots(ctx context.Context, root string) (Report, []SnapshotFile, error) {
	report, err := Verify(ctx, root)
	if err != nil {
		return Report{}, nil, err
	}
	if !report.OK {
		return report, nil, nil
	}
	manifest, err := readManifest(report.Manifest)
	if err != nil {
		return Report{}, nil, err
	}
	files := make([]SnapshotFile, 0, len(manifest.Snapshots))
	for _, snapshot := range manifest.Snapshots {
		clean := filepath.Clean(strings.TrimSpace(snapshot.Path))
		files = append(files, SnapshotFile{
			Path:      clean,
			Kind:      snapshot.Kind,
			CreatedAt: snapshot.CreatedAt,
			Records:   snapshot.Records,
			FullPath:  filepath.Join(report.Root, clean),
		})
	}
	return report, files, nil
}

func readManifest(path string) (Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read tenant store manifest: %w", err)
	}
	var raw struct {
		Version   string            `json:"version,omitempty"`
		Snapshots []json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Manifest{}, fmt.Errorf("decode tenant store manifest: %w", err)
	}
	manifest := Manifest{Version: raw.Version}
	for _, item := range raw.Snapshots {
		var snapshot Snapshot
		if err := json.Unmarshal(item, &snapshot); err == nil {
			manifest.Snapshots = append(manifest.Snapshots, snapshot)
			continue
		}
		var path string
		if err := json.Unmarshal(item, &path); err != nil {
			return Manifest{}, fmt.Errorf("decode tenant store snapshot entry: %w", err)
		}
		manifest.Snapshots = append(manifest.Snapshots, Snapshot{Path: path})
	}
	return manifest, nil
}

func validateSnapshotPath(root, path string) []Finding {
	var findings []Finding
	path = strings.TrimSpace(path)
	if path == "" {
		return []Finding{{Path: "manifest.json", Reason: "snapshot path is empty"}}
	}
	if filepath.IsAbs(path) {
		findings = append(findings, Finding{Path: path, Reason: "snapshot path must be relative"})
	}
	if hasUnsafePathText(path) {
		findings = append(findings, Finding{Path: path, Reason: "snapshot path contains unsafe characters or traversal"})
	}
	if !encryptedSnapshotPath(path) {
		findings = append(findings, Finding{Path: path, Reason: "snapshot must be compressed age output ending in .jsonl.zst.age or .tar.zst.age"})
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		findings = append(findings, Finding{Path: path, Reason: "snapshot path must stay inside the tenant store"})
	}
	full := filepath.Join(root, clean)
	if hasSymlinkComponent(root, clean) {
		findings = append(findings, Finding{Path: path, Reason: "snapshot path must not contain symlinks"})
	}
	info, err := os.Lstat(full)
	if err != nil {
		findings = append(findings, Finding{Path: path, Reason: "snapshot file is missing"})
		return findings
	}
	if info.Mode()&os.ModeSymlink != 0 {
		findings = append(findings, Finding{Path: path, Reason: "snapshot file must not be a symlink"})
	}
	if info.IsDir() {
		findings = append(findings, Finding{Path: path, Reason: "snapshot path must be a file"})
	}
	return findings
}

func scanStoreRoot(ctx context.Context, root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			findings = append(findings, Finding{Path: relPath(root, path), Reason: err.Error()})
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel := relPath(root, path)
		if rel == "." || rel == "manifest.json" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if gitIgnored(ctx, root, rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 {
			if !allowedDocSymlink(path, rel) {
				findings = append(findings, Finding{Path: rel, Reason: "tenant store must not contain symlinks outside the CLAUDE.md to AGENTS.md doc alias"})
			}
			return nil
		}
		if entry.IsDir() {
			if runtimeDirectoryName(name) {
				findings = append(findings, Finding{Path: rel, Reason: "tenant store must not contain plaintext runtime or report directories"})
				return filepath.SkipDir
			}
			return nil
		}
		if plaintextArtifact(rel) {
			findings = append(findings, Finding{Path: rel, Reason: "tenant store must not contain plaintext archive artifacts"})
		}
		if plaintextRuntimeFile(rel) {
			findings = append(findings, Finding{Path: rel, Reason: "tenant store must not contain plaintext runtime or report artifacts"})
		}
		return nil
	})
	return findings, err
}

func allowedDocSymlink(path, rel string) bool {
	if filepath.ToSlash(rel) != "CLAUDE.md" {
		return false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	return filepath.ToSlash(filepath.Clean(target)) == "AGENTS.md"
}

func gitIgnored(ctx context.Context, root, rel string) bool {
	if rel == "." || strings.TrimSpace(rel) == "" {
		return false
	}
	worktreeRoot, err := gitWorktreeRoot(ctx, root)
	if err != nil || canonicalPath(worktreeRoot) != canonicalPath(root) {
		return false
	}
	// #nosec G204 -- root resolved from verified store path; rel is filepath-cleaned; argv form.
	cmd := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "-q", "--", rel)
	return cmd.Run() == nil
}

func gitWorktreeRoot(ctx context.Context, root string) (string, error) {
	// #nosec G204 -- root resolved from verified store path; argv form, no shell.
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

func canonicalPath(path string) string {
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return clean
	}
	return filepath.Clean(resolved)
}

func (report *Report) add(findings ...Finding) {
	report.Findings = append(report.Findings, findings...)
}

func encryptedSnapshotPath(path string) bool {
	return strings.HasSuffix(path, ".jsonl.zst.age") || strings.HasSuffix(path, ".tar.zst.age")
}

func runtimeDirectoryName(name string) bool {
	switch name {
	case "logs", "reports", "screenshots", "state", "transcripts":
		return true
	default:
		return false
	}
}

func plaintextArtifact(path string) bool {
	lower := strings.ToLower(path)
	if encryptedSnapshotPath(lower) {
		return false
	}
	for _, suffix := range []string{
		".jsonl", ".jsonl.zst", ".tar", ".tar.zst",
		".db", ".db-wal", ".db-shm", ".db-journal",
		".sqlite", ".sqlite-wal", ".sqlite-shm", ".sqlite-journal",
		".sqlite3", ".sqlite3-wal", ".sqlite3-shm", ".sqlite3-journal",
	} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func plaintextRuntimeFile(path string) bool {
	lower := strings.ToLower(path)
	base := filepath.Base(lower)
	if strings.HasSuffix(base, ".log") || strings.HasSuffix(base, ".har") {
		return true
	}
	if strings.Contains(base, "report") && strings.HasSuffix(base, ".json") {
		return true
	}
	if strings.Contains(base, "screenshot") {
		for _, suffix := range []string{".png", ".jpg", ".jpeg", ".webp", ".pdf"} {
			if strings.HasSuffix(base, suffix) {
				return true
			}
		}
	}
	if strings.Contains(base, "transcript") {
		for _, suffix := range []string{".json", ".txt", ".md"} {
			if strings.HasSuffix(base, suffix) {
				return true
			}
		}
	}
	return false
}

func hasSymlinkComponent(root, clean string) bool {
	current := root
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func hasUnsafePathText(path string) bool {
	if strings.ContainsAny(path, "?#") {
		return true
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return true
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return true
		}
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
