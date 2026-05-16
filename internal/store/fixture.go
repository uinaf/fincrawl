package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ckstore "github.com/openclaw/crawlkit/store"
)

const ProviderIntercom = "intercom"

type Fixture struct {
	Workspace     Workspace      `json:"workspace"`
	Conversations []Conversation `json:"conversations"`
}

type Workspace struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type Conversation struct {
	ID           string         `json:"id"`
	Provider     string         `json:"provider"`
	ProviderID   string         `json:"provider_id"`
	Subject      string         `json:"subject"`
	State        string         `json:"state"`
	Assignee     string         `json:"assignee"`
	Rating       string         `json:"rating"`
	FinStatus    string         `json:"fin_status"`
	Participants []string       `json:"participants"`
	Tags         []string       `json:"tags"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
	Parts        []Part         `json:"parts"`
	Raw          map[string]any `json:"raw"`
}

type Part struct {
	ID         string         `json:"id"`
	ProviderID string         `json:"provider_id"`
	Type       string         `json:"type"`
	AuthorName string         `json:"author_name"`
	Body       string         `json:"body"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
	Raw        map[string]any `json:"raw"`
}

type SyncResult struct {
	WorkspaceID       string `json:"workspace_id"`
	Conversations     int    `json:"conversations"`
	ConversationParts int    `json:"conversation_parts"`
	RawBlobs          int    `json:"raw_blobs"`
}

func LoadFixture(path string) (Fixture, error) {
	fixturePath := filepath.Join(path, "conversations.json")
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture %s: %w", fixturePath, err)
	}
	var fixture Fixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		return Fixture{}, fmt.Errorf("decode fixture %s: %w", fixturePath, err)
	}
	normalizeFixture(&fixture)
	return fixture, nil
}

func SyncFixture(ctx context.Context, dbPath string, fixture Fixture) (SyncResult, error) {
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: Schema, SchemaVersion: SchemaVersion})
	if err != nil {
		return SyncResult{}, err
	}
	defer st.Close()
	result := SyncResult{WorkspaceID: fixture.Workspace.ID}
	err = st.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `insert into workspaces(id, provider, name, created_at) values(?, ?, ?, ?)
			on conflict(id) do update set provider=excluded.provider, name=excluded.name, created_at=excluded.created_at`,
			fixture.Workspace.ID, fixture.Workspace.Provider, fixture.Workspace.Name, fixture.Workspace.CreatedAt); err != nil {
			return fmt.Errorf("upsert workspace: %w", err)
		}
		for _, conversation := range fixture.Conversations {
			if err := upsertConversation(ctx, tx, fixture.Workspace.ID, conversation, &result); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func normalizeFixture(fixture *Fixture) {
	if fixture.Workspace.Provider == "" {
		fixture.Workspace.Provider = ProviderIntercom
	}
	for i := range fixture.Conversations {
		c := &fixture.Conversations[i]
		if c.Provider == "" {
			c.Provider = fixture.Workspace.Provider
		}
		sort.Strings(c.Tags)
		sort.Strings(c.Participants)
		for j := range c.Parts {
			if c.Parts[j].UpdatedAt == "" {
				c.Parts[j].UpdatedAt = c.Parts[j].CreatedAt
			}
		}
	}
	sort.Slice(fixture.Conversations, func(i, j int) bool {
		if fixture.Conversations[i].UpdatedAt == fixture.Conversations[j].UpdatedAt {
			return fixture.Conversations[i].ProviderID < fixture.Conversations[j].ProviderID
		}
		return fixture.Conversations[i].UpdatedAt < fixture.Conversations[j].UpdatedAt
	})
}

func upsertConversation(ctx context.Context, tx *sql.Tx, workspaceID string, conversation Conversation, result *SyncResult) error {
	if _, err := tx.ExecContext(ctx, `insert into conversations(
		id, workspace_id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
	) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	on conflict(id) do update set
		workspace_id=excluded.workspace_id,
		provider=excluded.provider,
		provider_id=excluded.provider_id,
		subject=excluded.subject,
		state=excluded.state,
		assignee=excluded.assignee,
		rating=excluded.rating,
		fin_status=excluded.fin_status,
		created_at=excluded.created_at,
		updated_at=excluded.updated_at`,
		conversation.ID, workspaceID, conversation.Provider, conversation.ProviderID, conversation.Subject,
		conversation.State, conversation.Assignee, conversation.Rating, conversation.FinStatus,
		conversation.CreatedAt, conversation.UpdatedAt); err != nil {
		return fmt.Errorf("upsert conversation %s: %w", conversation.ID, err)
	}
	result.Conversations++
	rawCount, err := insertRawBlob(ctx, tx, conversation.Provider, "conversation", conversation.ProviderID, conversation.Raw)
	if err != nil {
		return err
	}
	result.RawBlobs += rawCount
	if _, err := tx.ExecContext(ctx, `delete from conversation_tags where conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("clear tags: %w", err)
	}
	for _, tag := range conversation.Tags {
		tagID := "tag_" + stableID(tag)
		if _, err := tx.ExecContext(ctx, `insert into tags(id, name) values(?, ?) on conflict(id) do update set name=excluded.name`, tagID, tag); err != nil {
			return fmt.Errorf("upsert tag: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `insert into conversation_tags(conversation_id, tag_id) values(?, ?) on conflict do nothing`, conversation.ID, tagID); err != nil {
			return fmt.Errorf("link tag: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from conversation_parts where conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("clear parts: %w", err)
	}
	var bodies []string
	for _, part := range conversation.Parts {
		if _, err := tx.ExecContext(ctx, `insert into conversation_parts(
			id, conversation_id, provider, provider_id, part_type, author_name, body, created_at, updated_at
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			part.ID, conversation.ID, conversation.Provider, part.ProviderID, part.Type, part.AuthorName,
			part.Body, part.CreatedAt, part.UpdatedAt); err != nil {
			return fmt.Errorf("insert part %s: %w", part.ID, err)
		}
		result.ConversationParts++
		bodies = append(bodies, part.Body)
		rawCount, err := insertRawBlob(ctx, tx, conversation.Provider, "conversation_part", part.ProviderID, part.Raw)
		if err != nil {
			return err
		}
		result.RawBlobs += rawCount
	}
	if _, err := tx.ExecContext(ctx, `delete from conversation_fts where conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("clear fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into conversation_fts(conversation_id, subject, body, tags, participants, assignee) values(?, ?, ?, ?, ?, ?)`,
		conversation.ID, conversation.Subject, strings.Join(bodies, "\n"), strings.Join(conversation.Tags, " "), strings.Join(conversation.Participants, " "), conversation.Assignee); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}
	return nil
}

func insertRawBlob(ctx context.Context, tx *sql.Tx, provider, recordType, providerID string, raw map[string]any) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return 0, fmt.Errorf("marshal raw blob: %w", err)
	}
	hashBytes := sha256.Sum256(body)
	hash := hex.EncodeToString(hashBytes[:])
	result, err := tx.ExecContext(ctx, `insert into raw_blobs(hash, provider, record_type, provider_id, json, created_at)
		values(?, ?, ?, ?, ?, ?) on conflict(hash) do nothing`,
		hash, provider, recordType, providerID, string(body), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert raw blob: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count raw blob insert: %w", err)
	}
	return int(affected), nil
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:8])
}
