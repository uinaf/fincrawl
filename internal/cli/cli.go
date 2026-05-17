package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/openclaw/crawlkit/output"
	"github.com/uinaf/fincrawl/internal/archive"
	"github.com/uinaf/fincrawl/internal/buildinfo"
	"github.com/uinaf/fincrawl/internal/config"
	"github.com/uinaf/fincrawl/internal/control"
	"github.com/uinaf/fincrawl/internal/guard"
	"github.com/uinaf/fincrawl/internal/intercom"
	"github.com/uinaf/fincrawl/internal/lock"
	"github.com/uinaf/fincrawl/internal/store"
	"github.com/uinaf/fincrawl/internal/syncer"
)

type app struct {
	Doctor   doctorCmd   `cmd:"" help:"Check local configuration."`
	Metadata metadataCmd `cmd:"" help:"Print machine-readable metadata."`
	Describe describeCmd `cmd:"" help:"Print machine-readable command schemas."`
	Status   statusCmd   `cmd:"" help:"Print local archive status."`
	Sync     syncCmd     `cmd:"" help:"Sync conversations from fixtures or provider APIs."`
	Search   searchCmd   `cmd:"" help:"Search the local archive."`
	Archive  archiveCmd  `cmd:"" help:"Write compressed age-encrypted archive output."`
	Publish  publishCmd  `cmd:"" help:"Publish local SQLite state as an encrypted snapshot."`
	Import   importCmd   `cmd:"" help:"Import an encrypted snapshot into local SQLite."`
	Guard    guardCmd    `cmd:"" help:"Check commit guardrails."`
	Version  versionCmd  `cmd:"" help:"Print version information."`
}

type commandContext struct {
	context.Context
	stdout io.Writer
	stderr io.Writer
}

const liveHTTPTimeout = 90 * time.Second

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var cli app
	parser, err := kong.New(&cli,
		kong.Name("fincrawl"),
		kong.Description("Local-first support conversation archive."),
		kong.UsageOnError(),
	)
	if err != nil {
		return err
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		return output.UsageError{Err: err}
	}
	return kctx.Run(commandContext{Context: ctx, stdout: stdout, stderr: stderr})
}

type doctorCmd struct {
	Offline bool `help:"Do not attempt live provider calls."`
	JSON    bool `help:"Print JSON output." default:"true"`
}

type doctorResult struct {
	OK              bool              `json:"ok"`
	Offline         bool              `json:"offline"`
	ConfigPath      string            `json:"config_path"`
	DatabasePath    string            `json:"database_path"`
	Credentials     map[string]string `json:"credentials"`
	Checks          []checkResult     `json:"checks"`
	ComplianceNotes []string          `json:"compliance_notes"`
}

type checkResult struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func (cmd doctorCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	result := doctorResult{
		OK:           true,
		Offline:      cmd.Offline,
		ConfigPath:   rt.Paths.ConfigPath,
		DatabasePath: rt.Config.DBPath,
		Credentials: map[string]string{
			config.EnvAgeIdentity:  redactPresence(rt.AgeIdentitySet),
			config.EnvAgeRecipient: redactPresence(rt.AgeRecipientSet),
			config.EnvIntercomCred: redactPresence(rt.IntercomTokenSet),
		},
		ComplianceNotes: []string{
			"supported provider APIs and official exports only",
			"browser scraping and undocumented endpoint crawling are out of scope",
		},
	}
	if err := config.EnsureDirs(rt); err != nil {
		result.OK = false
		result.Checks = append(result.Checks, checkResult{Name: "runtime_dirs", OK: false, Detail: err.Error()})
	} else {
		result.Checks = append(result.Checks, checkResult{Name: "runtime_dirs", OK: true})
	}
	if recipient := config.AgeRecipient(); recipient != "" {
		if _, err := archive.ParseRecipient(recipient); err != nil {
			result.OK = false
			result.Checks = append(result.Checks, checkResult{Name: "age_recipient", OK: false, Detail: err.Error()})
		} else {
			result.Checks = append(result.Checks, checkResult{Name: "age_recipient", OK: true, Detail: "present"})
		}
	} else {
		result.Checks = append(result.Checks, checkResult{Name: "age_recipient", OK: true, Detail: "not set"})
	}
	if identity := config.AgeIdentity(); identity != "" {
		if _, err := archive.ParseIdentities(identity); err != nil {
			result.OK = false
			result.Checks = append(result.Checks, checkResult{Name: "age_identity", OK: false, Detail: err.Error()})
		} else {
			result.Checks = append(result.Checks, checkResult{Name: "age_identity", OK: true, Detail: "present"})
		}
	} else {
		result.Checks = append(result.Checks, checkResult{Name: "age_identity", OK: true, Detail: "not set"})
	}
	if cmd.JSON {
		if err := output.Write(ctx.stdout, output.JSON, "doctor", result); err != nil {
			return err
		}
		if !result.OK {
			return fmt.Errorf("doctor checks failed")
		}
		return nil
	}
	if !result.OK {
		return fmt.Errorf("doctor checks failed")
	}
	fmt.Fprintln(ctx.stdout, "ok")
	return nil
}

type metadataCmd struct {
	JSON bool `help:"Print JSON output." default:"true"`
}

func (cmd metadataCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	format := output.JSON
	if !cmd.JSON {
		format = output.Text
	}
	return output.Write(ctx.stdout, format, "metadata", control.Manifest(rt))
}

type describeCmd struct {
	Command string `arg:"" optional:"" help:"Optional command name to describe."`
	JSON    bool   `help:"Print JSON output." default:"true"`
}

func (cmd describeCmd) Run(ctx commandContext) error {
	schema, err := describeCommands(cmd.Command)
	if err != nil {
		return err
	}
	format := output.JSON
	if !cmd.JSON {
		format = output.Text
	}
	return output.Write(ctx.stdout, format, "describe", schema)
}

type statusCmd struct {
	JSON bool `help:"Print JSON output." default:"true"`
}

func (cmd statusCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	format := output.JSON
	if !cmd.JSON {
		format = output.Text
	}
	return output.Write(ctx.stdout, format, "status", control.Status(ctx, rt))
}

type syncCmd struct {
	Fixture       string `help:"Import synthetic fixture directory."`
	UpdatedSince  string `name:"updated-since" help:"Sync provider conversations updated since a duration or timestamp."`
	UpdatedBefore string `name:"updated-before" help:"Sync provider conversations updated before a duration or timestamp. Requires --updated-since."`
	Conversation  string `help:"Hydrate one provider conversation ID."`
	Entities      bool   `help:"Hydrate provider admins, teams, and tags."`
	Contacts      bool   `help:"Include a capped contact/user list when used with --entities."`
	Resume        bool   `help:"Resume an interrupted Intercom updated-since sync window."`
	Limit         int    `help:"Maximum provider conversations for --updated-since, or contacts for --entities --contacts. Use 0 for no conversation limit." default:"50"`
	DryRun        bool   `name:"dry-run" help:"Validate and describe planned sync work without writing local state or calling provider APIs."`
	JSON          bool   `help:"Print JSON output." default:"true"`
}

func (cmd syncCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if err := cmd.validateMode(); err != nil {
		return err
	}
	if cmd.Fixture != "" {
		fixture, err := store.LoadFixture(cmd.Fixture)
		if err != nil {
			return err
		}
		if cmd.DryRun {
			return writeMaybeJSON(ctx.stdout, cmd.JSON, syncDryRun(cmd.mode(), rt.Config.DBPath, false, fixtureCounts(fixture), nil))
		}
		if err := config.EnsureDirs(rt); err != nil {
			return err
		}
		lck, err := lock.Acquire(rt.Config.DBPath)
		if err != nil {
			return err
		}
		defer lck.Release()
		result, err := store.SyncFixture(ctx, rt.Config.DBPath, fixture)
		if err != nil {
			return err
		}
		return writeMaybeJSON(ctx.stdout, cmd.JSON, result)
	}
	if cmd.UpdatedSince != "" || cmd.Conversation != "" || cmd.Entities || cmd.Resume {
		now := time.Now().UTC()
		var updatedAfter time.Time
		updatedBefore := now
		if cmd.Conversation != "" {
			if err := validateProviderID("conversation", cmd.Conversation); err != nil {
				return output.UsageError{Err: err}
			}
		}
		if cmd.UpdatedSince != "" {
			updatedAfter, err = parseSince(cmd.UpdatedSince, now, "updated-since")
			if err != nil {
				return output.UsageError{Err: err}
			}
		}
		if cmd.UpdatedBefore != "" {
			updatedBefore, err = parseSince(cmd.UpdatedBefore, now, "updated-before")
			if err != nil {
				return output.UsageError{Err: err}
			}
			if !updatedBefore.After(updatedAfter) {
				return output.UsageError{Err: fmt.Errorf("updated-before must be after updated-since")}
			}
		}
		if cmd.DryRun {
			params := map[string]any{
				"contacts":     cmd.Contacts,
				"conversation": cmd.Conversation,
				"limit":        cmd.Limit,
			}
			if cmd.UpdatedSince != "" {
				params["updated_after"] = formatOptionalTime(updatedAfter)
				params["updated_before"] = formatOptionalTime(updatedBefore)
				params["updated_since"] = cmd.UpdatedSince
				params["updated_before_input"] = cmd.UpdatedBefore
			}
			plan := syncDryRun(cmd.mode(), rt.Config.DBPath, config.IntercomToken() != "", nil, params)
			return writeMaybeJSON(ctx.stdout, cmd.JSON, plan)
		}
		if err := config.EnsureDirs(rt); err != nil {
			return err
		}
		if config.IntercomToken() == "" {
			return fmt.Errorf("missing %s for live Intercom sync", config.EnvIntercomCred)
		}
		lck, err := lock.Acquire(rt.Config.DBPath)
		if err != nil {
			return err
		}
		defer lck.Release()
		client := intercom.Client{
			BaseURL:      config.IntercomBaseURL(),
			Token:        config.IntercomToken(),
			Version:      config.IntercomVersion(),
			HTTPClient:   &http.Client{Timeout: liveHTTPTimeout},
			MaxAttempts:  3,
			RetryBackoff: 2 * time.Second,
			Sleep: func(ctx context.Context, d time.Duration) error {
				timer := time.NewTimer(d)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-timer.C:
					return nil
				}
			},
		}
		s := syncer.IntercomSyncer{Client: client}
		var result store.SyncResult
		if cmd.Conversation != "" {
			result, err = s.SyncConversation(ctx, rt.Config.DBPath, cmd.Conversation)
		} else if cmd.Entities {
			result, err = s.SyncEntities(ctx, rt.Config.DBPath, syncer.EntitySyncOptions{IncludeContacts: cmd.Contacts, ContactLimit: cmd.Limit})
		} else if cmd.Resume {
			result, err = s.ResumeTail(ctx, rt.Config.DBPath, cmd.Limit)
		} else {
			result, err = s.SyncUpdatedSince(ctx, rt.Config.DBPath, updatedAfter, updatedBefore, cmd.Limit)
		}
		if err != nil {
			return err
		}
		return writeMaybeJSON(ctx.stdout, cmd.JSON, result)
	}
	panic("unreachable sync mode")
}

func (cmd syncCmd) mode() string {
	switch {
	case cmd.Fixture != "":
		return "fixture"
	case cmd.UpdatedSince != "":
		return "updated-since"
	case cmd.Conversation != "":
		return "conversation"
	case cmd.Entities:
		return "entities"
	case cmd.Resume:
		return "resume"
	default:
		return "unknown"
	}
}

func (cmd syncCmd) validateMode() error {
	modes := 0
	for _, enabled := range []bool{cmd.Fixture != "", cmd.UpdatedSince != "", cmd.Conversation != "", cmd.Entities, cmd.Resume} {
		if enabled {
			modes++
		}
	}
	if modes == 0 {
		return output.UsageError{Err: fmt.Errorf("sync requires --fixture, --updated-since, --conversation, --entities, or --resume")}
	}
	if modes > 1 {
		return output.UsageError{Err: fmt.Errorf("sync accepts exactly one of --fixture, --updated-since, --conversation, --entities, or --resume")}
	}
	if cmd.Contacts && !cmd.Entities {
		return output.UsageError{Err: fmt.Errorf("--contacts requires --entities")}
	}
	if cmd.UpdatedBefore != "" && cmd.UpdatedSince == "" {
		return output.UsageError{Err: fmt.Errorf("--updated-before requires --updated-since")}
	}
	if (cmd.UpdatedSince != "" || cmd.Resume || cmd.Entities) && cmd.Limit < 0 {
		return output.UsageError{Err: fmt.Errorf("--limit must be >= 0")}
	}
	return nil
}

func parseSince(value string, now time.Time, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days < 0 {
			return time.Time{}, fmt.Errorf("invalid day duration %q", value)
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour), nil
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration < 0 {
			return time.Time{}, fmt.Errorf("duration must be positive")
		}
		return now.Add(-duration), nil
	}
	if timestamp, err := time.Parse(time.RFC3339, value); err == nil {
		return timestamp.UTC(), nil
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(unix, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid %s %q; use a duration like 2h, 30d, RFC3339 timestamp, or unix seconds", field, value)
}

type searchCmd struct {
	Query     string `arg:"" help:"Search query."`
	Limit     int    `help:"Maximum results." default:"20"`
	State     string `help:"Filter by exact conversation state, such as open, closed, or snoozed."`
	FinStatus string `name:"fin-status" help:"Filter by exact Intercom-exposed Fin status."`
	Tag       string `help:"Filter by exact tag name."`
	Fields    string `help:"Comma-separated fields to include in JSON/text output."`
	NDJSON    bool   `name:"ndjson" help:"Print one JSON result per line."`
	JSON      bool   `help:"Print JSON output." default:"true"`
}

func (cmd searchCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if err := validateSearchFields(cmd.Fields); err != nil {
		return output.UsageError{Err: err}
	}
	results, err := store.SearchWithOptions(ctx, rt.Config.DBPath, cmd.Query, store.SearchOptions{
		Limit:     cmd.Limit,
		State:     cmd.State,
		FinStatus: cmd.FinStatus,
		Tag:       cmd.Tag,
	})
	if err != nil {
		return err
	}
	if cmd.NDJSON {
		return writeSearchNDJSON(ctx.stdout, results, cmd.Fields)
	}
	value, err := projectSearchResults(results, cmd.Fields)
	if err != nil {
		return output.UsageError{Err: err}
	}
	return writeMaybeJSON(ctx.stdout, cmd.JSON, value)
}

type archiveCmd struct {
	Fixture   string `help:"Archive synthetic fixture directory."`
	Recipient string `help:"Age recipient or SSH public key recipient. Defaults to FINCRAWL_AGE_RECIPIENT."`
	Out       string `help:"Output path ending in .jsonl.zst.age." required:""`
	DryRun    bool   `name:"dry-run" help:"Validate and describe archive output without writing an artifact."`
	JSON      bool   `help:"Print JSON output." default:"true"`
}

type archiveResult struct {
	Output  string `json:"output"`
	Records int    `json:"records"`
}

func (cmd archiveCmd) Run(ctx commandContext) error {
	_, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if cmd.Fixture == "" {
		return output.UsageError{Err: fmt.Errorf("archive currently requires --fixture")}
	}
	if err := validateArchiveArtifactPath("--out", cmd.Out); err != nil {
		return output.UsageError{Err: err}
	}
	recipient := strings.TrimSpace(cmd.Recipient)
	if recipient == "" {
		recipient = config.AgeRecipient()
	}
	fixture, err := store.LoadFixture(cmd.Fixture)
	if err != nil {
		return err
	}
	records := archive.FixtureRecords(fixture)
	if cmd.DryRun {
		if _, err := archive.ParseRecipient(recipient); err != nil {
			return err
		}
		return writeMaybeJSON(ctx.stdout, cmd.JSON, archiveDryRun("archive", cmd.Out, len(records)))
	}
	if err := os.MkdirAll(filepath.Dir(cmd.Out), 0o755); err != nil && filepath.Dir(cmd.Out) != "." {
		return err
	}
	if err := archive.WriteEncryptedJSONL(cmd.Out, recipient, records); err != nil {
		return err
	}
	return writeMaybeJSON(ctx.stdout, cmd.JSON, archiveResult{Output: cmd.Out, Records: len(records)})
}

type publishCmd struct {
	Recipient string `help:"Age recipient or SSH public key recipient. Defaults to FINCRAWL_AGE_RECIPIENT."`
	Out       string `help:"Output path ending in .jsonl.zst.age." required:""`
	DryRun    bool   `name:"dry-run" help:"Validate and describe publish output without writing an artifact."`
	JSON      bool   `help:"Print JSON output." default:"true"`
}

func (cmd publishCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if err := validateArchiveArtifactPath("--out", cmd.Out); err != nil {
		return output.UsageError{Err: err}
	}
	recipient := strings.TrimSpace(cmd.Recipient)
	if recipient == "" {
		recipient = config.AgeRecipient()
	}
	if _, err := archive.ParseRecipient(recipient); err != nil {
		return err
	}
	if cmd.DryRun {
		fixture, err := store.ExportFixture(ctx, rt.Config.DBPath)
		if err != nil {
			return err
		}
		records := archive.FixtureRecords(fixture)
		return writeMaybeJSON(ctx.stdout, cmd.JSON, archiveDryRun("publish", cmd.Out, len(records)))
	}
	lck, err := lock.Acquire(rt.Config.DBPath)
	if err != nil {
		return err
	}
	defer lck.Release()
	fixture, err := store.ExportFixture(ctx, rt.Config.DBPath)
	if err != nil {
		return err
	}
	records := archive.FixtureRecords(fixture)
	if err := os.MkdirAll(filepath.Dir(cmd.Out), 0o755); err != nil && filepath.Dir(cmd.Out) != "." {
		return err
	}
	if err := archive.WriteEncryptedJSONL(cmd.Out, recipient, records); err != nil {
		return err
	}
	return writeMaybeJSON(ctx.stdout, cmd.JSON, archiveResult{Output: cmd.Out, Records: len(records)})
}

type importCmd struct {
	Identity string `help:"Age identity. Defaults to FINCRAWL_AGE_IDENTITY."`
	In       string `help:"Input path ending in .jsonl.zst.age." required:""`
	DryRun   bool   `name:"dry-run" help:"Validate and describe import work without writing local state."`
	JSON     bool   `help:"Print JSON output." default:"true"`
}

type importResult struct {
	Input   string           `json:"input"`
	Records int              `json:"records"`
	Sync    store.SyncResult `json:"sync"`
}

func (cmd importCmd) Run(ctx commandContext) error {
	rt, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if err := validateArchiveArtifactPath("--in", cmd.In); err != nil {
		return output.UsageError{Err: err}
	}
	identity := strings.TrimSpace(cmd.Identity)
	if identity == "" {
		identity = config.AgeIdentity()
	}
	records, err := archive.ReadEncryptedJSONL(cmd.In, identity)
	if err != nil {
		return err
	}
	fixture, err := archive.RecordsFixture(records)
	if err != nil {
		return err
	}
	if cmd.DryRun {
		return writeMaybeJSON(ctx.stdout, cmd.JSON, importDryRun(cmd.In, rt.Config.DBPath, len(records)))
	}
	if err := config.EnsureDirs(rt); err != nil {
		return err
	}
	lck, err := lock.Acquire(rt.Config.DBPath)
	if err != nil {
		return err
	}
	defer lck.Release()
	result, err := store.SyncFixture(ctx, rt.Config.DBPath, fixture)
	if err != nil {
		return err
	}
	return writeMaybeJSON(ctx.stdout, cmd.JSON, importResult{Input: cmd.In, Records: len(records), Sync: result})
}

type guardCmd struct {
	JSON bool `help:"Print JSON output." default:"true"`
}

func (cmd guardCmd) Run(ctx commandContext) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := guard.Run(root)
	if err != nil {
		return err
	}
	if cmd.JSON {
		if err := output.Write(ctx.stdout, output.JSON, "guard", result); err != nil {
			return err
		}
	} else if result.OK {
		fmt.Fprintf(ctx.stdout, "ok (%d files scanned)\n", result.Scanned)
	} else {
		for _, finding := range result.Findings {
			fmt.Fprintf(ctx.stderr, "%s: %s\n", finding.Path, finding.Reason)
		}
	}
	if !result.OK {
		return fmt.Errorf("guard failed with %d finding(s)", len(result.Findings))
	}
	return nil
}

type versionCmd struct {
	JSON bool `help:"Print JSON output." default:"true"`
}

func (cmd versionCmd) Run(ctx commandContext) error {
	info := buildinfo.Current()
	if cmd.JSON {
		return output.Write(ctx.stdout, output.JSON, "version", info)
	}
	fmt.Fprintln(ctx.stdout, info.Version)
	return nil
}

func redactPresence(present bool) string {
	if present {
		return "present"
	}
	return "absent"
}

func writeMaybeJSON(w io.Writer, jsonOutput bool, value any) error {
	if jsonOutput {
		return output.Write(w, output.JSON, "result", value)
	}
	_, err := fmt.Fprintln(w, value)
	return err
}
