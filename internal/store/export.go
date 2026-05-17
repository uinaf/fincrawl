package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	ckstore "github.com/openclaw/crawlkit/store"
)

func ExportFixture(ctx context.Context, dbPath string) (Fixture, error) {
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return Fixture{}, err
	}
	defer st.Close()
	db := st.DB()
	workspace, err := exportWorkspace(ctx, db)
	if err != nil {
		return Fixture{}, err
	}
	fixture := Fixture{Workspace: workspace}
	rawBlobs, err := exportRawBlobs(ctx, db)
	if err != nil {
		return Fixture{}, err
	}
	if fixture.Entities, err = exportEntities(ctx, db, rawBlobs); err != nil {
		return Fixture{}, err
	}
	if fixture.Conversations, err = exportConversations(ctx, db, rawBlobs); err != nil {
		return Fixture{}, err
	}
	normalizeFixture(&fixture)
	return fixture, nil
}

func exportWorkspace(ctx context.Context, db *sql.DB) (Workspace, error) {
	exists, err := tableExists(ctx, db, "workspaces")
	if err != nil {
		return Workspace{}, err
	}
	if !exists {
		return exportInferredWorkspace(ctx, db)
	}
	var workspace Workspace
	err = db.QueryRowContext(ctx, `select id, provider, name, created_at from workspaces order by id limit 1`).
		Scan(&workspace.ID, &workspace.Provider, &workspace.Name, &workspace.CreatedAt)
	if err == sql.ErrNoRows {
		return Workspace{}, fmt.Errorf("archive database has no workspace")
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("export workspace: %w", err)
	}
	return workspace, nil
}

func exportInferredWorkspace(ctx context.Context, db *sql.DB) (Workspace, error) {
	var workspace Workspace
	err := db.QueryRowContext(ctx, `select workspace_id, provider, min(created_at) from conversations group by workspace_id, provider order by workspace_id limit 1`).
		Scan(&workspace.ID, &workspace.Provider, &workspace.CreatedAt)
	if err == sql.ErrNoRows {
		return Workspace{}, fmt.Errorf("archive database has no workspace")
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("infer workspace: %w", err)
	}
	workspace.Name = workspace.ID
	return workspace, nil
}

type rawBlobKey struct {
	Provider   string
	RecordType string
	ProviderID string
}

func exportEntities(ctx context.Context, db *sql.DB, rawBlobs map[rawBlobKey]map[string]any) (Entities, error) {
	var entities Entities
	exists, err := tableExists(ctx, db, "admins")
	if err != nil {
		return Entities{}, err
	}
	if exists {
		admins, err := db.QueryContext(ctx, `select id, provider, provider_id, name, email, team_ids from admins order by provider, provider_id`)
		if err != nil {
			return Entities{}, fmt.Errorf("export admins: %w", err)
		}
		defer admins.Close()
		for admins.Next() {
			var admin Admin
			var teamIDs string
			if err := admins.Scan(&admin.ID, &admin.Provider, &admin.ProviderID, &admin.Name, &admin.Email, &teamIDs); err != nil {
				return Entities{}, fmt.Errorf("scan admin: %w", err)
			}
			admin.TeamIDs = splitCSV(teamIDs)
			admin.Raw = rawBlobs[rawBlobKey{Provider: admin.Provider, RecordType: "admin", ProviderID: admin.ProviderID}]
			entities.Admins = append(entities.Admins, admin)
		}
		if err := admins.Err(); err != nil {
			return Entities{}, err
		}
	}

	exists, err = tableExists(ctx, db, "teams")
	if err != nil {
		return Entities{}, err
	}
	if exists {
		teams, err := db.QueryContext(ctx, `select id, provider, provider_id, name from teams order by provider, provider_id`)
		if err != nil {
			return Entities{}, fmt.Errorf("export teams: %w", err)
		}
		defer teams.Close()
		for teams.Next() {
			var team Team
			if err := teams.Scan(&team.ID, &team.Provider, &team.ProviderID, &team.Name); err != nil {
				return Entities{}, fmt.Errorf("scan team: %w", err)
			}
			team.Raw = rawBlobs[rawBlobKey{Provider: team.Provider, RecordType: "team", ProviderID: team.ProviderID}]
			entities.Teams = append(entities.Teams, team)
		}
		if err := teams.Err(); err != nil {
			return Entities{}, err
		}
	}

	exists, err = tableExists(ctx, db, "provider_tags")
	if err != nil {
		return Entities{}, err
	}
	if exists {
		tags, err := db.QueryContext(ctx, `select id, provider, provider_id, name from provider_tags order by provider, provider_id`)
		if err != nil {
			return Entities{}, fmt.Errorf("export provider tags: %w", err)
		}
		defer tags.Close()
		for tags.Next() {
			var tag ProviderTag
			if err := tags.Scan(&tag.ID, &tag.Provider, &tag.ProviderID, &tag.Name); err != nil {
				return Entities{}, fmt.Errorf("scan provider tag: %w", err)
			}
			tag.Raw = rawBlobs[rawBlobKey{Provider: tag.Provider, RecordType: "tag", ProviderID: tag.ProviderID}]
			entities.Tags = append(entities.Tags, tag)
		}
		if err := tags.Err(); err != nil {
			return Entities{}, err
		}
	}

	exists, err = tableExists(ctx, db, "contacts")
	if err != nil {
		return Entities{}, err
	}
	if exists {
		contacts, err := db.QueryContext(ctx, `select id, provider, provider_id, name, email from contacts order by provider, provider_id`)
		if err != nil {
			return Entities{}, fmt.Errorf("export contacts: %w", err)
		}
		defer contacts.Close()
		for contacts.Next() {
			var contact Contact
			if err := contacts.Scan(&contact.ID, &contact.Provider, &contact.ProviderID, &contact.Name, &contact.Email); err != nil {
				return Entities{}, fmt.Errorf("scan contact: %w", err)
			}
			contact.Raw = rawBlobs[rawBlobKey{Provider: contact.Provider, RecordType: "contact", ProviderID: contact.ProviderID}]
			entities.Contacts = append(entities.Contacts, contact)
		}
		if err := contacts.Err(); err != nil {
			return Entities{}, err
		}
	}
	return entities, nil
}

func exportConversations(ctx context.Context, db *sql.DB, rawBlobs map[rawBlobKey]map[string]any) ([]Conversation, error) {
	rows, err := db.QueryContext(ctx, `select id, provider, provider_id, subject, state, assignee, rating, fin_status, created_at, updated_at
		from conversations order by updated_at, provider, provider_id`)
	if err != nil {
		return nil, fmt.Errorf("export conversations: %w", err)
	}
	var conversations []Conversation
	for rows.Next() {
		var conversation Conversation
		if err := rows.Scan(
			&conversation.ID,
			&conversation.Provider,
			&conversation.ProviderID,
			&conversation.Subject,
			&conversation.State,
			&conversation.Assignee,
			&conversation.Rating,
			&conversation.FinStatus,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, conversation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range conversations {
		conversation := &conversations[i]
		conversation.Participants, err = exportParticipants(ctx, db, conversation.ID)
		if err != nil {
			return nil, err
		}
		conversation.Tags, err = exportConversationTags(ctx, db, conversation.ID)
		if err != nil {
			return nil, err
		}
		conversation.Parts, err = exportParts(ctx, db, conversation.ID, rawBlobs)
		if err != nil {
			return nil, err
		}
		conversation.Raw = rawBlobs[rawBlobKey{Provider: conversation.Provider, RecordType: "conversation", ProviderID: conversation.ProviderID}]
	}
	return conversations, nil
}

func exportParticipants(ctx context.Context, db *sql.DB, conversationID string) ([]string, error) {
	exists, err := tableExists(ctx, db, "conversation_participants")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `select name from conversation_participants where conversation_id = ? order by name`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("export participants: %w", err)
	}
	defer rows.Close()
	var participants []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan participant: %w", err)
		}
		participants = append(participants, name)
	}
	return participants, rows.Err()
}

func exportConversationTags(ctx context.Context, db *sql.DB, conversationID string) ([]string, error) {
	tagsExist, err := tableExists(ctx, db, "tags")
	if err != nil {
		return nil, err
	}
	conversationTagsExist, err := tableExists(ctx, db, "conversation_tags")
	if err != nil {
		return nil, err
	}
	if !tagsExist || !conversationTagsExist {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `select t.name from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = ? order by t.name`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("export conversation tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan conversation tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func exportParts(ctx context.Context, db *sql.DB, conversationID string, rawBlobs map[rawBlobKey]map[string]any) ([]Part, error) {
	rows, err := db.QueryContext(ctx, `select id, provider, provider_id, part_type, author_name, body, created_at, updated_at
		from conversation_parts where conversation_id = ? order by updated_at, provider_id`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("export conversation parts: %w", err)
	}
	defer rows.Close()
	var parts []Part
	for rows.Next() {
		var part Part
		var provider string
		if err := rows.Scan(&part.ID, &provider, &part.ProviderID, &part.Type, &part.AuthorName, &part.Body, &part.CreatedAt, &part.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation part: %w", err)
		}
		part.Raw = rawBlobs[rawBlobKey{Provider: provider, RecordType: "conversation_part", ProviderID: part.ProviderID}]
		parts = append(parts, part)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return parts, nil
}

func exportRawBlobs(ctx context.Context, db *sql.DB) (map[rawBlobKey]map[string]any, error) {
	exists, err := tableExists(ctx, db, "raw_blobs")
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[rawBlobKey]map[string]any{}, nil
	}
	rows, err := db.QueryContext(ctx, `select provider, record_type, provider_id, json from raw_blobs order by provider, record_type, provider_id, created_at, hash`)
	if err != nil {
		return nil, fmt.Errorf("export raw blobs: %w", err)
	}
	defer rows.Close()
	rawBlobs := map[rawBlobKey]map[string]any{}
	for rows.Next() {
		var key rawBlobKey
		var body string
		if err := rows.Scan(&key.Provider, &key.RecordType, &key.ProviderID, &body); err != nil {
			return nil, fmt.Errorf("scan raw blob: %w", err)
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			return nil, fmt.Errorf("decode raw blob %s/%s/%s: %w", key.Provider, key.RecordType, key.ProviderID, err)
		}
		rawBlobs[key] = raw
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rawBlobs, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	fields := strings.Split(value, ",")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	sort.Strings(out)
	return out
}
