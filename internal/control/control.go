package control

import (
	"context"
	"os"

	ckcontrol "github.com/openclaw/crawlkit/control"
	"github.com/uinaf/fincrawl/internal/config"
	"github.com/uinaf/fincrawl/internal/store"
)

const syntheticArchiveRecipient = "age1n9zrm0rcxehv7cm55uqw27v9cguz4ev5dtyl7kxkn3vdpvap94ds2gn6rl"

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
		"doctor":   {Title: "Check local configuration", Argv: []string{"fincrawl", "doctor", "--offline", "--json"}, JSON: true},
		"metadata": {Title: "Print machine-readable metadata", Argv: []string{"fincrawl", "metadata", "--json"}, JSON: true},
		"status":   {Title: "Print archive status", Argv: []string{"fincrawl", "status", "--json"}, JSON: true},
		"sync":     {Title: "Sync conversations", Argv: []string{"fincrawl", "sync", "--fixture", "testdata/synthetic"}, Mutates: true},
		"search":   {Title: "Search local archive", Argv: []string{"fincrawl", "search", "query", "--json"}, JSON: true},
		"archive":  {Title: "Write encrypted archive", Argv: []string{"fincrawl", "archive", "--fixture", "testdata/synthetic", "--recipient", syntheticArchiveRecipient, "--out", "tmp/fincrawl-smoke.jsonl.zst.age"}, Mutates: true},
		"guard":    {Title: "Check commit guardrails", Argv: []string{"fincrawl", "guard", "--json"}, JSON: true},
	}
	manifest.Capabilities = []string{"intercom-api-sync", "sqlite-fts-search", "zstd-age-archive", "synthetic-fixtures"}
	manifest.Privacy = ckcontrol.Privacy{
		ContainsPrivateMessages: true,
		ExportsSecrets:          false,
		LocalOnlyScopes:         []string{"tenant-credentials", "tenant-artifacts"},
	}
	return manifest
}

func Status(ctx context.Context, rt config.Runtime) ckcontrol.Status {
	status := ckcontrol.NewStatus(config.AppID, "local archive status")
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
		return status
	}
	counts, err := store.Counts(ctx, rt.Config.DBPath)
	if err != nil {
		status.State = "warning"
		status.Warnings = append(status.Warnings, err.Error())
		return status
	}
	status.Counts = []ckcontrol.Count{
		ckcontrol.NewCount("conversations", "Conversations", counts.Conversations),
		ckcontrol.NewCount("conversation_parts", "Conversation parts", counts.ConversationParts),
		ckcontrol.NewCount("raw_blobs", "Raw blobs", counts.RawBlobs),
	}
	status.Databases = []ckcontrol.Database{
		ckcontrol.SQLiteDatabase("primary", "Primary archive", "archive", rt.Config.DBPath, true, status.Counts),
	}
	return status
}
