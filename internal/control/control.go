package control

import (
	"context"
	"os"
	"strings"

	ckcontrol "github.com/openclaw/crawlkit/control"
	"github.com/uinaf/fincrawl/internal/config"
	"github.com/uinaf/fincrawl/internal/store"
)

const syntheticArchiveRecipient = "age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl"

type StatusReport struct {
	ckcontrol.Status
	SyncStates []SyncStateStatus `json:"sync_states,omitempty"`
}

type SyncStateStatus struct {
	ID                string `json:"id"`
	Provider          string `json:"provider"`
	CursorKind        string `json:"cursor_kind"`
	State             string `json:"state"`
	ResumeAvailable   bool   `json:"resume_available"`
	HighWaterMark     string `json:"high_water_mark,omitempty"`
	ActiveWindowStart string `json:"active_window_start,omitempty"`
	ActiveWindowEnd   string `json:"active_window_end,omitempty"`
	HasLastProviderID bool   `json:"has_last_provider_id,omitempty"`
	HasPageCursor     bool   `json:"has_page_cursor,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

func Manifest(rt config.Runtime) ckcontrol.Manifest {
	manifest := ckcontrol.NewManifest(config.AppID, config.DisplayName, "fincrawl")
	manifest.Description = "Local-first support conversation archive"
	manifest.Paths = ckcontrol.Paths{
		DefaultConfig:   rt.Paths.ConfigPath,
		ConfigEnv:       config.App.ConfigEnv,
		DefaultDatabase: rt.Config.DBPath,
		DefaultCache:    rt.Config.CacheDir,
		DefaultLogs:     rt.Config.LogDir,
		DefaultShare:    rt.Config.ShareDir,
	}
	manifest.Commands = map[string]ckcontrol.Command{
		"doctor":   {Title: "Check local configuration", Argv: []string{"fincrawl", "doctor", "--offline"}, JSON: true},
		"metadata": {Title: "Print machine-readable metadata", Argv: []string{"fincrawl", "metadata"}, JSON: true},
		"describe": {Title: "Print command schemas", Argv: []string{"fincrawl", "describe"}, JSON: true},
		"status":   {Title: "Print archive status", Argv: []string{"fincrawl", "status"}, JSON: true},
		"sync":     {Title: "Sync conversations", Argv: []string{"fincrawl", "sync", "--fixture", "testdata/synthetic", "--dry-run"}, JSON: true, Mutates: true},
		"search":   {Title: "Search local archive", Argv: []string{"fincrawl", "search", "query", "--fields", "provider_id,subject,score,updated_at"}, JSON: true},
		"show":     {Title: "Show one local conversation", Argv: []string{"fincrawl", "show", "provider-conversation-id", "--fields", "provider_id,subject,tags,snippet"}, JSON: true},
		"archive":  {Title: "Write encrypted archive", Argv: []string{"fincrawl", "archive", "--fixture", "testdata/synthetic", "--recipient", syntheticArchiveRecipient, "--out", "tmp/fincrawl-smoke.jsonl.zst.age", "--dry-run"}, JSON: true, Mutates: true},
		"publish":  {Title: "Publish encrypted snapshot", Argv: []string{"fincrawl", "publish", "--recipient", syntheticArchiveRecipient, "--out", "tmp/fincrawl-publish.jsonl.zst.age", "--dry-run"}, JSON: true, Mutates: true},
		"import":   {Title: "Import encrypted snapshot", Argv: []string{"fincrawl", "import", "--in", "tmp/fincrawl-publish.jsonl.zst.age", "--dry-run"}, JSON: true, Mutates: true},
		"store":    {Title: "Verify encrypted tenant store", Argv: []string{"fincrawl", "store", "verify", "."}, JSON: true},
		"guard":    {Title: "Check commit guardrails", Argv: []string{"fincrawl", "guard"}, JSON: true},
	}
	manifest.Capabilities = []string{"intercom-api-sync", "sqlite-fts-search", "conversation-show", "zstd-age-archive", "encrypted-snapshot-import", "tenant-store-verify", "synthetic-fixtures"}
	manifest.Privacy = ckcontrol.Privacy{
		ContainsPrivateMessages: true,
		ExportsSecrets:          false,
		LocalOnlyScopes:         []string{"tenant-credentials", "tenant-artifacts"},
	}
	return manifest
}

func Status(ctx context.Context, rt config.Runtime) StatusReport {
	report := StatusReport{Status: ckcontrol.NewStatus(config.AppID, "local archive status")}
	status := &report.Status
	status.State = "ready"
	status.ConfigPath = rt.Paths.ConfigPath
	status.DatabasePath = rt.Config.DBPath
	status.Databases = []ckcontrol.Database{
		ckcontrol.SQLiteDatabase("primary", "Primary archive", "archive", rt.Config.DBPath, true, nil),
	}
	if _, err := os.Stat(rt.Config.DBPath); err != nil {
		status.State = "empty"
		status.Summary = "archive database has not been created"
		if !os.IsNotExist(err) {
			status.Warnings = append(status.Warnings, err.Error())
		}
		return report
	}
	counts, err := store.Counts(ctx, rt.Config.DBPath)
	if err != nil {
		status.State = "warning"
		status.Warnings = append(status.Warnings, err.Error())
		return report
	}
	status.Counts = []ckcontrol.Count{
		ckcontrol.NewCount("conversations", "Conversations", counts.Conversations),
		ckcontrol.NewCount("conversation_parts", "Conversation parts", counts.ConversationParts),
		ckcontrol.NewCount("admins", "Admins", counts.Admins),
		ckcontrol.NewCount("teams", "Teams", counts.Teams),
		ckcontrol.NewCount("tags", "Provider tags", counts.Tags),
		ckcontrol.NewCount("contacts", "Contacts", counts.Contacts),
		ckcontrol.NewCount("raw_blobs", "Raw blobs", counts.RawBlobs),
	}
	status.Databases = []ckcontrol.Database{
		ckcontrol.SQLiteDatabase("primary", "Primary archive", "archive", rt.Config.DBPath, true, status.Counts),
	}
	states, err := store.ListSyncStates(ctx, rt.Config.DBPath)
	if err != nil {
		status.State = "warning"
		status.Warnings = append(status.Warnings, err.Error())
		return report
	}
	report.SyncStates = syncStateStatuses(states)
	return report
}

func syncStateStatuses(states []store.SyncState) []SyncStateStatus {
	results := make([]SyncStateStatus, 0, len(states))
	for _, state := range states {
		active := strings.TrimSpace(state.ActiveWindowStart) != "" || strings.TrimSpace(state.ActiveWindowEnd) != ""
		status := SyncStateStatus{
			ID:                state.ID,
			Provider:          state.Provider,
			CursorKind:        state.CursorKind,
			State:             "idle",
			ResumeAvailable:   active,
			HighWaterMark:     state.HighWaterMark,
			ActiveWindowStart: state.ActiveWindowStart,
			ActiveWindowEnd:   state.ActiveWindowEnd,
			HasLastProviderID: strings.TrimSpace(state.LastProviderID) != "",
			HasPageCursor:     strings.TrimSpace(state.PageCursor) != "",
			UpdatedAt:         state.UpdatedAt,
		}
		if active {
			status.State = "active"
		}
		results = append(results, status)
	}
	return results
}
