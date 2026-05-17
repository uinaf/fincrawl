package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ProviderIntercom = "intercom"

type Fixture struct {
	Workspace     Workspace      `json:"workspace"`
	Entities      Entities       `json:"entities,omitempty"`
	Conversations []Conversation `json:"conversations"`
}

type Workspace struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type Entities struct {
	Admins   []Admin       `json:"admins,omitempty"`
	Teams    []Team        `json:"teams,omitempty"`
	Tags     []ProviderTag `json:"tags,omitempty"`
	Contacts []Contact     `json:"contacts,omitempty"`
}

type Admin struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider"`
	ProviderID string         `json:"provider_id"`
	Name       string         `json:"name"`
	Email      string         `json:"email,omitempty"`
	TeamIDs    []string       `json:"team_ids,omitempty"`
	Raw        map[string]any `json:"raw,omitempty"`
}

type Team struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider"`
	ProviderID string         `json:"provider_id"`
	Name       string         `json:"name"`
	Raw        map[string]any `json:"raw,omitempty"`
}

type ProviderTag struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider"`
	ProviderID string         `json:"provider_id"`
	Name       string         `json:"name"`
	Raw        map[string]any `json:"raw,omitempty"`
}

type Contact struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider"`
	ProviderID string         `json:"provider_id"`
	Name       string         `json:"name"`
	Email      string         `json:"email,omitempty"`
	Raw        map[string]any `json:"raw,omitempty"`
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
	WorkspaceID       string   `json:"workspace_id"`
	Conversations     int      `json:"conversations"`
	ConversationParts int      `json:"conversation_parts"`
	Admins            int      `json:"admins,omitempty"`
	Teams             int      `json:"teams,omitempty"`
	Tags              int      `json:"tags,omitempty"`
	Contacts          int      `json:"contacts,omitempty"`
	RawBlobs          int      `json:"raw_blobs"`
	Warnings          []string `json:"warnings,omitempty"`
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
	result, err := SyncEntities(ctx, dbPath, fixture.Workspace, fixture.Entities)
	if err != nil {
		return SyncResult{}, err
	}
	conversations, err := SyncConversations(ctx, dbPath, fixture.Workspace, fixture.Conversations)
	if err != nil {
		return SyncResult{}, err
	}
	result.Conversations += conversations.Conversations
	result.ConversationParts += conversations.ConversationParts
	result.RawBlobs += conversations.RawBlobs
	if result.WorkspaceID == "" {
		result.WorkspaceID = conversations.WorkspaceID
	}
	return result, nil
}

func SyncConversations(ctx context.Context, dbPath string, workspace Workspace, conversations []Conversation) (SyncResult, error) {
	st, err := openStore(ctx, dbPath)
	if err != nil {
		return SyncResult{}, err
	}
	defer st.Close()
	workspace = normalizeWorkspace(workspace)
	result := SyncResult{WorkspaceID: workspace.ID}
	err = st.WithTx(ctx, func(tx *sql.Tx) error {
		if err := upsertWorkspace(ctx, tx, workspace); err != nil {
			return err
		}
		for _, conversation := range conversations {
			if conversation.Provider == "" {
				conversation.Provider = workspace.Provider
			}
			if err := upsertConversation(ctx, tx, workspace.ID, conversation, &result); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func SyncEntities(ctx context.Context, dbPath string, workspace Workspace, entities Entities) (SyncResult, error) {
	st, err := openStore(ctx, dbPath)
	if err != nil {
		return SyncResult{}, err
	}
	defer st.Close()
	workspace = normalizeWorkspace(workspace)
	result := SyncResult{WorkspaceID: workspace.ID}
	err = st.WithTx(ctx, func(tx *sql.Tx) error {
		if err := upsertWorkspace(ctx, tx, workspace); err != nil {
			return err
		}
		for _, admin := range entities.Admins {
			if admin.Provider == "" {
				admin.Provider = workspace.Provider
			}
			if admin.ID == "" {
				admin.ID = "admin_" + stableID(admin.Provider+":"+admin.ProviderID)
			}
			if _, err := tx.ExecContext(ctx, `insert into admins(id, provider, provider_id, name, email, team_ids) values(?, ?, ?, ?, ?, ?)
				on conflict(provider, provider_id) do update set id=excluded.id, name=excluded.name, email=excluded.email, team_ids=excluded.team_ids`,
				admin.ID, admin.Provider, admin.ProviderID, admin.Name, admin.Email, strings.Join(admin.TeamIDs, ",")); err != nil {
				return fmt.Errorf("upsert admin %s: %w", admin.ProviderID, err)
			}
			result.Admins++
			rawCount, err := insertRawBlob(ctx, tx, admin.Provider, "admin", admin.ProviderID, admin.Raw)
			if err != nil {
				return err
			}
			result.RawBlobs += rawCount
		}
		for _, team := range entities.Teams {
			if team.Provider == "" {
				team.Provider = workspace.Provider
			}
			if team.ID == "" {
				team.ID = "team_" + stableID(team.Provider+":"+team.ProviderID)
			}
			if _, err := tx.ExecContext(ctx, `insert into teams(id, provider, provider_id, name) values(?, ?, ?, ?)
				on conflict(provider, provider_id) do update set id=excluded.id, name=excluded.name`,
				team.ID, team.Provider, team.ProviderID, team.Name); err != nil {
				return fmt.Errorf("upsert team %s: %w", team.ProviderID, err)
			}
			result.Teams++
			rawCount, err := insertRawBlob(ctx, tx, team.Provider, "team", team.ProviderID, team.Raw)
			if err != nil {
				return err
			}
			result.RawBlobs += rawCount
		}
		for _, tag := range entities.Tags {
			if tag.Provider == "" {
				tag.Provider = workspace.Provider
			}
			if tag.ID == "" {
				tag.ID = "provider_tag_" + stableID(tag.Provider+":"+tag.ProviderID)
			}
			if _, err := tx.ExecContext(ctx, `insert into provider_tags(id, provider, provider_id, name) values(?, ?, ?, ?)
				on conflict(provider, provider_id) do update set id=excluded.id, name=excluded.name`,
				tag.ID, tag.Provider, tag.ProviderID, tag.Name); err != nil {
				return fmt.Errorf("upsert provider tag %s: %w", tag.ProviderID, err)
			}
			result.Tags++
			rawCount, err := insertRawBlob(ctx, tx, tag.Provider, "tag", tag.ProviderID, tag.Raw)
			if err != nil {
				return err
			}
			result.RawBlobs += rawCount
		}
		for _, contact := range entities.Contacts {
			if contact.Provider == "" {
				contact.Provider = workspace.Provider
			}
			if contact.ID == "" {
				contact.ID = "contact_" + stableID(contact.Provider+":"+contact.ProviderID)
			}
			if _, err := tx.ExecContext(ctx, `insert into contacts(id, provider, provider_id, name, email) values(?, ?, ?, ?, ?)
				on conflict(provider, provider_id) do update set id=excluded.id, name=excluded.name, email=excluded.email`,
				contact.ID, contact.Provider, contact.ProviderID, contact.Name, contact.Email); err != nil {
				return fmt.Errorf("upsert contact %s: %w", contact.ProviderID, err)
			}
			result.Contacts++
			rawCount, err := insertRawBlob(ctx, tx, contact.Provider, "contact", contact.ProviderID, contact.Raw)
			if err != nil {
				return err
			}
			result.RawBlobs += rawCount
		}
		return nil
	})
	return result, err
}

func normalizeWorkspace(workspace Workspace) Workspace {
	if workspace.Provider == "" {
		workspace.Provider = ProviderIntercom
	}
	if workspace.ID == "" {
		workspace.ID = workspace.Provider
	}
	if workspace.Name == "" {
		workspace.Name = workspace.ID
	}
	if workspace.CreatedAt == "" {
		workspace.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return workspace
}

func upsertWorkspace(ctx context.Context, tx *sql.Tx, workspace Workspace) error {
	if _, err := tx.ExecContext(ctx, `insert into workspaces(id, provider, name, created_at) values(?, ?, ?, ?)
		on conflict(id) do update set provider=excluded.provider, name=excluded.name, created_at=excluded.created_at`,
		workspace.ID, workspace.Provider, workspace.Name, workspace.CreatedAt); err != nil {
		return fmt.Errorf("upsert workspace: %w", err)
	}
	return nil
}

func normalizeFixture(fixture *Fixture) {
	if fixture.Workspace.Provider == "" {
		fixture.Workspace.Provider = ProviderIntercom
	}
	normalizeEntities(&fixture.Entities, fixture.Workspace.Provider)
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

func normalizeEntities(entities *Entities, provider string) {
	for i := range entities.Admins {
		admin := &entities.Admins[i]
		if admin.Provider == "" {
			admin.Provider = provider
		}
		if admin.ID == "" {
			admin.ID = "admin_" + stableID(admin.Provider+":"+admin.ProviderID)
		}
		if admin.Name == "" {
			admin.Name = firstNonEmpty(admin.Email, admin.ProviderID)
		}
		sort.Strings(admin.TeamIDs)
	}
	for i := range entities.Teams {
		team := &entities.Teams[i]
		if team.Provider == "" {
			team.Provider = provider
		}
		if team.ID == "" {
			team.ID = "team_" + stableID(team.Provider+":"+team.ProviderID)
		}
		if team.Name == "" {
			team.Name = team.ProviderID
		}
	}
	for i := range entities.Tags {
		tag := &entities.Tags[i]
		if tag.Provider == "" {
			tag.Provider = provider
		}
		if tag.ID == "" {
			tag.ID = "provider_tag_" + stableID(tag.Provider+":"+tag.ProviderID)
		}
		if tag.Name == "" {
			tag.Name = tag.ProviderID
		}
	}
	for i := range entities.Contacts {
		contact := &entities.Contacts[i]
		if contact.Provider == "" {
			contact.Provider = provider
		}
		if contact.ID == "" {
			contact.ID = "contact_" + stableID(contact.Provider+":"+contact.ProviderID)
		}
		if contact.Name == "" {
			contact.Name = firstNonEmpty(contact.Email, contact.ProviderID)
		}
	}
	sort.Slice(entities.Admins, func(i, j int) bool { return entities.Admins[i].ProviderID < entities.Admins[j].ProviderID })
	sort.Slice(entities.Teams, func(i, j int) bool { return entities.Teams[i].ProviderID < entities.Teams[j].ProviderID })
	sort.Slice(entities.Tags, func(i, j int) bool { return entities.Tags[i].ProviderID < entities.Tags[j].ProviderID })
	sort.Slice(entities.Contacts, func(i, j int) bool { return entities.Contacts[i].ProviderID < entities.Contacts[j].ProviderID })
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
	if _, err := tx.ExecContext(ctx, `delete from conversation_participants where conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("clear participants: %w", err)
	}
	for _, participant := range conversation.Participants {
		if strings.TrimSpace(participant) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `insert into conversation_participants(conversation_id, name) values(?, ?) on conflict do nothing`, conversation.ID, participant); err != nil {
			return fmt.Errorf("link participant: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from conversation_parts where conversation_id = ?`, conversation.ID); err != nil {
		return fmt.Errorf("clear parts: %w", err)
	}
	movedPartConversationIDs := map[string]struct{}{}
	for _, part := range dedupeParts(conversation.ID, conversation.Parts) {
		previousConversationID, err := existingPartConversationID(ctx, tx, conversation.Provider, part.ProviderID, conversation.ID)
		if err != nil {
			return err
		}
		if previousConversationID != "" {
			movedPartConversationIDs[previousConversationID] = struct{}{}
		}
		if _, err := tx.ExecContext(ctx, `insert into conversation_parts(
			id, conversation_id, provider, provider_id, part_type, author_name, body, created_at, updated_at
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(provider, provider_id) do update set
			id=excluded.id,
			conversation_id=excluded.conversation_id,
			part_type=excluded.part_type,
			author_name=excluded.author_name,
			body=excluded.body,
			created_at=excluded.created_at,
			updated_at=excluded.updated_at`,
			part.ID, conversation.ID, conversation.Provider, part.ProviderID, part.Type, part.AuthorName,
			part.Body, part.CreatedAt, part.UpdatedAt); err != nil {
			return fmt.Errorf("upsert part %s: %w", part.ID, err)
		}
		result.ConversationParts++
		rawCount, err := insertRawBlob(ctx, tx, conversation.Provider, "conversation_part", part.ProviderID, part.Raw)
		if err != nil {
			return err
		}
		result.RawBlobs += rawCount
	}
	if err := rebuildConversationFTS(ctx, tx, conversation.ID); err != nil {
		return err
	}
	for movedConversationID := range movedPartConversationIDs {
		if err := rebuildConversationFTS(ctx, tx, movedConversationID); err != nil {
			return err
		}
	}
	return nil
}

func existingPartConversationID(ctx context.Context, tx *sql.Tx, provider, providerID, currentConversationID string) (string, error) {
	var conversationID string
	err := tx.QueryRowContext(ctx, `select conversation_id from conversation_parts where provider = ? and provider_id = ? and conversation_id <> ?`, provider, providerID, currentConversationID).Scan(&conversationID)
	if err == nil {
		return conversationID, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return "", fmt.Errorf("lookup existing part %s: %w", providerID, err)
}

func rebuildConversationFTS(ctx context.Context, tx *sql.Tx, conversationID string) error {
	if _, err := tx.ExecContext(ctx, `delete from conversation_fts where conversation_id = ?`, conversationID); err != nil {
		return fmt.Errorf("clear fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into conversation_fts(conversation_id, subject, body, tags, participants, assignee, state, rating, fin_status)
		select c.id,
			c.subject,
			coalesce((select group_concat(body, char(10)) from (select p.body as body from conversation_parts p where p.conversation_id = c.id order by p.updated_at, p.provider_id)), ''),
			coalesce((select group_concat(name, ' ') from (select t.name as name from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id order by t.name)), ''),
			coalesce((select group_concat(name, ' ') from (select cp.name as name from conversation_participants cp where cp.conversation_id = c.id order by cp.name)), ''),
			c.assignee,
			c.state,
			c.rating,
			c.fin_status
		from conversations c
		where c.id = ?`, conversationID); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}
	return nil
}

func dedupeParts(conversationID string, parts []Part) []Part {
	seen := make(map[string]int, len(parts))
	deduped := make([]Part, 0, len(parts))
	for index, part := range parts {
		part = normalizePartIdentity(conversationID, part, index)
		key := part.ProviderID
		if existing, ok := seen[key]; ok {
			deduped[existing] = part
			continue
		}
		seen[key] = len(deduped)
		deduped = append(deduped, part)
	}
	return deduped
}

func normalizePartIdentity(conversationID string, part Part, index int) Part {
	if strings.TrimSpace(part.ProviderID) == "" {
		part.ProviderID = strings.TrimSpace(part.ID)
	}
	if strings.TrimSpace(part.ProviderID) == "" {
		part.ProviderID = fmt.Sprintf("%s:part:%d", conversationID, index+1)
	}
	if strings.TrimSpace(part.ID) == "" {
		part.ID = "part_" + stableID(part.ProviderID)
	}
	return part
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
