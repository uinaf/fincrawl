package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/openclaw/crawlkit/output"
	"github.com/uinaf/fincrawl/internal/store"
)

const commandSchemaVersion = "fincrawl.cli.v1"

type cliSchema struct {
	SchemaVersion string                   `json:"schema_version"`
	Commands      map[string]commandSchema `json:"commands"`
}

type commandSchema struct {
	Name     string        `json:"name"`
	Summary  string        `json:"summary"`
	Mutates  bool          `json:"mutates"`
	JSON     bool          `json:"json"`
	Args     []paramSchema `json:"args,omitempty"`
	Flags    []paramSchema `json:"flags,omitempty"`
	Examples []string      `json:"examples,omitempty"`
	Notes    []string      `json:"notes,omitempty"`
}

type paramSchema struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Default  string `json:"default,omitempty"`
	Help     string `json:"help"`
}

type errorEnvelope struct {
	OK    bool      `json:"ok"`
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Usage   bool   `json:"usage"`
}

type dryRunPlan struct {
	DryRun             bool           `json:"dry_run"`
	Command            string         `json:"command"`
	Mode               string         `json:"mode,omitempty"`
	WouldMutate        bool           `json:"would_mutate"`
	LiveRequest        bool           `json:"live_request,omitempty"`
	CredentialRequired bool           `json:"credential_required,omitempty"`
	CredentialPresent  bool           `json:"credential_present,omitempty"`
	DatabasePath       string         `json:"database_path,omitempty"`
	Output             string         `json:"output,omitempty"`
	Input              string         `json:"input,omitempty"`
	Records            int            `json:"records,omitempty"`
	Counts             map[string]int `json:"counts,omitempty"`
	Parameters         map[string]any `json:"parameters,omitempty"`
	WouldRead          []string       `json:"would_read,omitempty"`
	WouldWrite         []string       `json:"would_write,omitempty"`
	WouldCall          []string       `json:"would_call,omitempty"`
}

func describeCommands(command string) (cliSchema, error) {
	command = strings.TrimSpace(command)
	schema := cliSchema{
		SchemaVersion: commandSchemaVersion,
		Commands: map[string]commandSchema{
			"doctor": {
				Name:    "doctor",
				Summary: "Check local configuration and redact credential presence.",
				JSON:    true,
				Flags: []paramSchema{
					{Name: "offline", Type: "bool", Help: "Do not attempt live provider calls."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl doctor --offline"},
			},
			"metadata": {
				Name:    "metadata",
				Summary: "Print machine-readable app metadata and control manifest.",
				JSON:    true,
				Flags: []paramSchema{
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl metadata --json"},
			},
			"describe": {
				Name:    "describe",
				Summary: "Print machine-readable command schemas.",
				JSON:    true,
				Args: []paramSchema{
					{Name: "command", Type: "string", Help: "Optional command name to describe."},
				},
				Flags: []paramSchema{
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl describe --json", "fincrawl describe search --json"},
			},
			"status": {
				Name:    "status",
				Summary: "Print local archive status, database counts, and sync cursors.",
				JSON:    true,
				Flags: []paramSchema{
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl status --json"},
			},
			"sync": {
				Name:    "sync",
				Summary: "Sync conversations from synthetic fixtures or official provider APIs.",
				Mutates: true,
				JSON:    true,
				Flags: []paramSchema{
					{Name: "fixture", Type: "path", Help: "Import synthetic fixture directory."},
					{Name: "updated-since", Type: "duration|timestamp", Help: "Sync provider conversations updated since a duration or timestamp."},
					{Name: "updated-before", Type: "duration|timestamp", Help: "Sync provider conversations updated before a duration or timestamp. Requires --updated-since."},
					{Name: "conversation", Type: "provider-id", Help: "Hydrate one provider conversation ID."},
					{Name: "entities", Type: "bool", Help: "Hydrate provider admins, teams, and tags."},
					{Name: "contacts", Type: "bool", Help: "Include a capped contact/user list when used with --entities."},
					{Name: "resume", Type: "bool", Help: "Resume an interrupted Intercom updated-since sync window."},
					{Name: "limit", Type: "int", Default: "50", Help: "Maximum provider conversations for --updated-since, or contacts for --entities --contacts. Use 0 for no conversation limit."},
					{Name: "dry-run", Type: "bool", Help: "Validate and describe planned sync work without writing local state or calling provider APIs."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{
					"fincrawl sync --fixture testdata/synthetic",
					"fincrawl sync --updated-since 2h --limit 50 --dry-run",
					"fincrawl sync --updated-since 180d --updated-before 90d --limit 0 --dry-run",
					"fincrawl sync --conversation <provider-conversation-id> --dry-run",
				},
				Notes: []string{
					"Exactly one sync mode is required.",
					"Live sync requires FINCRAWL_INTERCOM_TOKEN.",
					"Provider IDs reject whitespace, path traversal, query fragments, and percent-encoded path separators.",
				},
			},
			"search": {
				Name:    "search",
				Summary: "Search the local SQLite archive.",
				JSON:    true,
				Args: []paramSchema{
					{Name: "query", Type: "string", Required: true, Help: "Search query."},
				},
				Flags: []paramSchema{
					{Name: "limit", Type: "int", Default: "20", Help: "Maximum results."},
					{Name: "state", Type: "string", Help: "Filter by exact conversation state, such as open, closed, or snoozed."},
					{Name: "fin-status", Type: "string", Help: "Filter by exact Intercom-exposed Fin status."},
					{Name: "tag", Type: "string", Help: "Filter by exact tag name."},
					{Name: "fields", Type: "field-list", Help: "Comma-separated fields to include in output."},
					{Name: "ndjson", Type: "bool", Help: "Print one JSON result per line."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{
					"fincrawl search \"billing refund\" --limit 10",
					"fincrawl search \"billing refund\" --state open --tag billing",
					"fincrawl search \"login\" --fin-status resolved",
					"fincrawl search \"billing refund\" --fields provider_id,subject,score,updated_at --ndjson",
				},
				Notes: []string{"Allowed fields: id, provider_id, subject, state, assignee, rating, fin_status, participants, tags, updated_at, snippet, score."},
			},
			"show": {
				Name:    "show",
				Summary: "Show one local conversation by local ID or provider ID.",
				JSON:    true,
				Args: []paramSchema{
					{Name: "id", Type: "conversation-id|provider-id", Required: true, Help: "Conversation local ID or provider ID."},
				},
				Flags: []paramSchema{
					{Name: "fields", Type: "field-list", Help: "Comma-separated fields to include in output."},
					{Name: "parts", Type: "bool", Help: "Include sanitized conversation parts."},
					{Name: "part-limit", Type: "int", Default: "20", Help: "Maximum parts when --parts is set."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{
					"fincrawl show <provider-conversation-id>",
					"fincrawl show <provider-conversation-id> --fields provider_id,subject,tags,snippet",
					"fincrawl show <provider-conversation-id> --parts --part-limit 5",
				},
				Notes: []string{"Allowed fields: id, provider_id, subject, state, assignee, rating, fin_status, participants, tags, created_at, updated_at, snippet, parts.", "Parts are opt-in and sanitized before output."},
			},
			"archive": {
				Name:    "archive",
				Summary: "Write compressed age-encrypted archive output.",
				Mutates: true,
				JSON:    true,
				Flags: []paramSchema{
					{Name: "fixture", Type: "path", Help: "Archive synthetic fixture directory."},
					{Name: "recipient", Type: "age-recipient|ssh-public-key", Help: "Age recipient or SSH public key recipient. Defaults to FINCRAWL_AGE_RECIPIENT."},
					{Name: "out", Type: "relative-path", Required: true, Help: "Output path under the current repo ending in .jsonl.zst.age."},
					{Name: "dry-run", Type: "bool", Help: "Validate and describe archive output without writing an artifact."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl archive --fixture testdata/synthetic --recipient age1... --out tmp/snapshot.jsonl.zst.age --dry-run"},
				Notes:    []string{"Output paths must be relative, stay under the current working directory, and end in .jsonl.zst.age."},
			},
			"publish": {
				Name:    "publish",
				Summary: "Publish local SQLite state as a compressed age-encrypted snapshot.",
				Mutates: true,
				JSON:    true,
				Flags: []paramSchema{
					{Name: "recipient", Type: "age-recipient|ssh-public-key", Help: "Age recipient or SSH public key recipient. Defaults to FINCRAWL_AGE_RECIPIENT."},
					{Name: "out", Type: "relative-path", Required: true, Help: "Output path under the current repo ending in .jsonl.zst.age."},
					{Name: "dry-run", Type: "bool", Help: "Validate and describe publish output without writing an artifact."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl publish --out snapshots/local.jsonl.zst.age --dry-run"},
				Notes: []string{
					"Reads the local SQLite archive and writes only compressed age-encrypted JSONL.",
					"Output paths must be relative, stay under the current working directory, and end in .jsonl.zst.age.",
				},
			},
			"import": {
				Name:    "import",
				Summary: "Import a compressed age-encrypted snapshot into local SQLite.",
				Mutates: true,
				JSON:    true,
				Flags: []paramSchema{
					{Name: "identity", Type: "age-identity", Help: "Age identity. Defaults to FINCRAWL_AGE_IDENTITY."},
					{Name: "in", Type: "relative-path", Required: true, Help: "Input path under the current repo ending in .jsonl.zst.age."},
					{Name: "dry-run", Type: "bool", Help: "Validate and describe import work without writing local state."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl import --in snapshots/local.jsonl.zst.age --dry-run"},
				Notes: []string{
					"Decrypts and imports into local SQLite; it does not call live provider APIs.",
					"Input paths must be relative, stay under the current working directory, and end in .jsonl.zst.age.",
				},
			},
			"subscribe": {
				Name:    "subscribe",
				Summary: "Import snapshots from a local encrypted tenant store.",
				Mutates: true,
				JSON:    true,
				Args: []paramSchema{
					{Name: "store", Type: "path", Required: true, Help: "Local tenant store root containing manifest.json."},
				},
				Flags: []paramSchema{
					{Name: "identity", Type: "age-identity", Help: "Age identity. Defaults to FINCRAWL_AGE_IDENTITY."},
					{Name: "dry-run", Type: "bool", Help: "Verify and list planned imports without writing local state."},
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl subscribe ../tenant-store --dry-run", "fincrawl subscribe ../tenant-store --identity AGE-SECRET-KEY-..."},
				Notes: []string{
					"Local filesystem stores only; remote clone, pull, and scheduling are out of scope.",
					"The tenant store is verified before import, and this command currently imports .jsonl.zst.age snapshots.",
				},
			},
			"store verify": {
				Name:    "store verify",
				Summary: "Verify a generic encrypted tenant-store manifest and artifact boundary.",
				JSON:    true,
				Args: []paramSchema{
					{Name: "path", Type: "path", Default: ".", Help: "Tenant store root containing manifest.json."},
				},
				Flags: []paramSchema{
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl store verify .", "fincrawl store verify ../tenant-store --json"},
				Notes: []string{
					"Manifest snapshots must reference existing .jsonl.zst.age or .tar.zst.age files with relative paths.",
					"Plaintext archives, local databases, runtime state, logs, reports, screenshots, and transcripts are rejected.",
				},
			},
			"guard": {
				Name:    "guard",
				Summary: "Check commit guardrails for tenant data, plaintext archives, and secret-like values.",
				JSON:    true,
				Flags: []paramSchema{
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl guard"},
			},
			"version": {
				Name:    "version",
				Summary: "Print version information.",
				JSON:    true,
				Flags: []paramSchema{
					{Name: "json", Type: "bool", Default: "true", Help: "Print JSON output."},
				},
				Examples: []string{"fincrawl version"},
			},
		},
	}
	if command == "" {
		return schema, nil
	}
	if command == "store" {
		command = "store verify"
	}
	described, ok := schema.Commands[command]
	if !ok {
		return cliSchema{}, output.UsageError{Err: fmt.Errorf("unknown command %q", command)}
	}
	schema.Commands = map[string]commandSchema{command: described}
	return schema, nil
}

func WantsJSON(args []string) bool {
	wants := true
	for _, arg := range args {
		switch {
		case arg == "--json":
			wants = true
		case arg == "--no-json":
			wants = false
		case strings.HasPrefix(arg, "--json="):
			value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--json="))
			if err != nil {
				wants = true
			} else {
				wants = value
			}
		case arg == "--format=json" || arg == "--output=json":
			wants = true
		case arg == "--format=text" || arg == "--output=text":
			wants = false
		}
	}
	return wants
}

func WriteError(w io.Writer, err error, args []string) int {
	if err == nil {
		return 0
	}
	usage := output.IsUsage(err)
	code := "runtime_error"
	exitCode := 1
	if usage {
		code = "usage_error"
		exitCode = 2
	}
	if WantsJSON(args) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(errorEnvelope{
			OK: false,
			Error: errorBody{
				Code:    code,
				Message: err.Error(),
				Usage:   usage,
			},
		})
		return exitCode
	}
	_, _ = fmt.Fprintln(w, err)
	return exitCode
}

func syncDryRun(mode, dbPath string, credentialPresent bool, counts map[string]int, params map[string]any) dryRunPlan {
	plan := dryRunPlan{
		DryRun:       true,
		Command:      "sync",
		Mode:         mode,
		WouldMutate:  true,
		DatabasePath: dbPath,
		Counts:       counts,
		Parameters:   compactParams(params),
		WouldWrite:   []string{dbPath},
	}
	if mode != "fixture" {
		plan.LiveRequest = true
		plan.CredentialRequired = true
		plan.CredentialPresent = credentialPresent
		plan.WouldCall = []string{"intercom"}
	}
	return plan
}

func archiveDryRun(command, out string, records int) dryRunPlan {
	return dryRunPlan{
		DryRun:      true,
		Command:     command,
		WouldMutate: true,
		Output:      out,
		Records:     records,
		WouldWrite:  []string{out},
	}
}

func importDryRun(in, dbPath string, records int) dryRunPlan {
	return dryRunPlan{
		DryRun:       true,
		Command:      "import",
		WouldMutate:  true,
		Input:        in,
		Records:      records,
		DatabasePath: dbPath,
		WouldRead:    []string{in},
		WouldWrite:   []string{dbPath},
	}
}

func fixtureCounts(fixture store.Fixture) map[string]int {
	parts := 0
	for _, conversation := range fixture.Conversations {
		parts += len(conversation.Parts)
	}
	return map[string]int{
		"admins":        len(fixture.Entities.Admins),
		"contacts":      len(fixture.Entities.Contacts),
		"conversations": len(fixture.Conversations),
		"parts":         parts,
		"tags":          len(fixture.Entities.Tags),
		"teams":         len(fixture.Entities.Teams),
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func compactParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	compact := make(map[string]any, len(params))
	for key, value := range params {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				compact[key] = typed
			}
		default:
			compact[key] = value
		}
	}
	if len(compact) == 0 {
		return nil
	}
	return compact
}

func validateProviderID(kind, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s ID is required", kind)
	}
	if id != strings.TrimSpace(id) {
		return fmt.Errorf("%s ID must not have leading or trailing whitespace", kind)
	}
	if len(id) > 256 {
		return fmt.Errorf("%s ID is too long", kind)
	}
	if hasControlOrSpace(id) {
		return fmt.Errorf("%s ID must not contain whitespace or control characters", kind)
	}
	if strings.ContainsAny(id, `/\?#`) {
		return fmt.Errorf("%s ID must not contain path separators, query strings, or fragments", kind)
	}
	lower := strings.ToLower(id)
	if strings.Contains(lower, "..") || strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return fmt.Errorf("%s ID must not contain path traversal or encoded path separators", kind)
	}
	return nil
}

func validateArchiveOut(path string) error {
	return validateArchiveArtifactPath("--out", path)
}

func validateArchiveArtifactPath(flag, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", flag)
	}
	if path != strings.TrimSpace(path) {
		return fmt.Errorf("%s must not have leading or trailing whitespace", flag)
	}
	if !strings.HasSuffix(path, ".jsonl.zst.age") {
		return fmt.Errorf("%s must end in .jsonl.zst.age", flag)
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("%s must be relative to the current working directory", flag)
	}
	if hasControl(path) {
		return fmt.Errorf("%s must not contain control characters", flag)
	}
	if strings.ContainsAny(path, "?#") {
		return fmt.Errorf("%s must not contain query strings or fragments", flag)
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return fmt.Errorf("%s must not contain percent-encoded path traversal or separators", flag)
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("%s must stay under the current working directory", flag)
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return fmt.Errorf("%s must not contain path traversal", flag)
		}
	}
	return nil
}

func projectSearchResults(results []store.SearchResult, fields string) (any, error) {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return results, nil
	}
	names, err := parseSearchFields(fields)
	if err != nil {
		return nil, err
	}
	projected := make([]map[string]any, 0, len(results))
	for _, result := range results {
		row := make(map[string]any, len(names))
		for _, name := range names {
			value, _ := searchResultField(result, name)
			row[name] = value
		}
		projected = append(projected, row)
	}
	return projected, nil
}

func writeSearchNDJSON(w io.Writer, results []store.SearchResult, fields string) error {
	enc := json.NewEncoder(w)
	projected, err := projectSearchResults(results, fields)
	if err != nil {
		return err
	}
	switch rows := projected.(type) {
	case []store.SearchResult:
		for _, row := range rows {
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
	case []map[string]any:
		for _, row := range rows {
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected search projection type %T", projected)
	}
	return nil
}

func validateSearchFields(fields string) error {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return nil
	}
	_, err := parseSearchFields(fields)
	return err
}

func parseSearchFields(fields string) ([]string, error) {
	rawNames := strings.Split(fields, ",")
	names := make([]string, 0, len(rawNames))
	for _, name := range rawNames {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("--fields contains an empty field")
		}
		if _, err := searchResultField(store.SearchResult{}, name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func searchResultField(result store.SearchResult, name string) (any, error) {
	switch name {
	case "id":
		return result.ID, nil
	case "provider_id":
		return result.ProviderID, nil
	case "subject":
		return result.Subject, nil
	case "state":
		return result.State, nil
	case "assignee":
		return result.Assignee, nil
	case "rating":
		return result.Rating, nil
	case "fin_status":
		return result.FinStatus, nil
	case "participants":
		return result.Participants, nil
	case "tags":
		return result.Tags, nil
	case "updated_at":
		return result.UpdatedAt, nil
	case "snippet":
		return result.Snippet, nil
	case "score":
		return result.Score, nil
	default:
		return nil, fmt.Errorf("unknown search field %q", name)
	}
}

func projectConversationDetail(detail store.ConversationDetail, fields string) (any, error) {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return detail, nil
	}
	names, err := parseConversationFields(fields)
	if err != nil {
		return nil, err
	}
	projected := make(map[string]any, len(names))
	for _, name := range names {
		value, _ := conversationDetailField(detail, name)
		projected[name] = value
	}
	return projected, nil
}

func validateConversationFields(fields string) error {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return nil
	}
	_, err := parseConversationFields(fields)
	return err
}

func parseConversationFields(fields string) ([]string, error) {
	rawNames := strings.Split(fields, ",")
	names := make([]string, 0, len(rawNames))
	for _, name := range rawNames {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("--fields contains an empty field")
		}
		if _, err := conversationDetailField(store.ConversationDetail{}, name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func conversationDetailField(detail store.ConversationDetail, name string) (any, error) {
	switch name {
	case "id":
		return detail.ID, nil
	case "provider_id":
		return detail.ProviderID, nil
	case "subject":
		return detail.Subject, nil
	case "state":
		return detail.State, nil
	case "assignee":
		return detail.Assignee, nil
	case "rating":
		return detail.Rating, nil
	case "fin_status":
		return detail.FinStatus, nil
	case "participants":
		return detail.Participants, nil
	case "tags":
		return detail.Tags, nil
	case "created_at":
		return detail.CreatedAt, nil
	case "updated_at":
		return detail.UpdatedAt, nil
	case "snippet":
		return detail.Snippet, nil
	case "parts":
		return detail.Parts, nil
	default:
		return nil, fmt.Errorf("unknown show field %q", name)
	}
}

func hasControlOrSpace(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
