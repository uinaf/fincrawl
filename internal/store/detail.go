package store

import (
	"context"
	"database/sql"
	"fmt"

	ckstore "github.com/openclaw/crawlkit/store"
)

type ConversationDetailOptions struct {
	IncludeParts bool
	PartLimit    int
}

type ConversationDetail struct {
	ID           string                   `json:"id"`
	ProviderID   string                   `json:"provider_id"`
	Subject      string                   `json:"subject"`
	State        string                   `json:"state"`
	Assignee     string                   `json:"assignee,omitempty"`
	Rating       string                   `json:"rating,omitempty"`
	FinStatus    string                   `json:"fin_status,omitempty"`
	Participants []string                 `json:"participants,omitempty"`
	Tags         []string                 `json:"tags,omitempty"`
	CreatedAt    string                   `json:"created_at"`
	UpdatedAt    string                   `json:"updated_at"`
	Snippet      string                   `json:"snippet"`
	Parts        []ConversationPartDetail `json:"parts,omitempty"`
}

type ConversationPartDetail struct {
	ID         string `json:"id"`
	ProviderID string `json:"provider_id"`
	Type       string `json:"type"`
	AuthorName string `json:"author_name,omitempty"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func GetConversation(ctx context.Context, dbPath, id string, opts ConversationDetailOptions) (ConversationDetail, error) {
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return ConversationDetail{}, err
	}
	defer st.Close()
	detail, err := getConversationDetail(ctx, st.DB(), id)
	if err != nil {
		return ConversationDetail{}, err
	}
	if opts.IncludeParts {
		parts, err := getConversationParts(ctx, st.DB(), detail.ID, opts.PartLimit)
		if err != nil {
			return ConversationDetail{}, err
		}
		detail.Parts = parts
	}
	return detail, nil
}

func getConversationDetail(ctx context.Context, db *sql.DB, id string) (ConversationDetail, error) {
	hasParticipantTable, err := tableExists(ctx, db, "conversation_participants")
	if err != nil {
		return ConversationDetail{}, err
	}
	hasFTSTable, err := tableExists(ctx, db, "conversation_fts")
	if err != nil {
		return ConversationDetail{}, err
	}
	hasConversationTagsTable, err := tableExists(ctx, db, "conversation_tags")
	if err != nil {
		return ConversationDetail{}, err
	}
	hasTagsTable, err := tableExists(ctx, db, "tags")
	if err != nil {
		return ConversationDetail{}, err
	}
	hasPartsTable, err := tableExists(ctx, db, "conversation_parts")
	if err != nil {
		return ConversationDetail{}, err
	}
	participantsSelect := `coalesce(
		nullif((select group_concat(cp.name, ', ') from conversation_participants cp where cp.conversation_id = c.id), ''),
		(select f.participants from conversation_fts f where f.conversation_id = c.id),
		''
	)`
	if hasParticipantTable && !hasFTSTable {
		participantsSelect = `coalesce(nullif((select group_concat(cp.name, ', ') from conversation_participants cp where cp.conversation_id = c.id), ''), '')`
	} else if !hasParticipantTable && hasFTSTable {
		participantsSelect = `coalesce((select f.participants from conversation_fts f where f.conversation_id = c.id), '')`
	} else if !hasParticipantTable {
		participantsSelect = `''`
	}
	tagsSelect := `''`
	if hasConversationTagsTable && hasTagsTable {
		tagsSelect = `coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '')`
	}
	snippetSelect := `c.subject`
	if hasPartsTable {
		snippetSelect = `substr(coalesce((select group_concat(p.body, ' ') from conversation_parts p where p.conversation_id = c.id), c.subject), 1, 480)`
	}
	var detail ConversationDetail
	var participants string
	var tags string
	var snippet string
	err = db.QueryRowContext(ctx, fmt.Sprintf(`select c.id, c.provider_id, c.subject, c.state, c.assignee, c.rating, c.fin_status, c.created_at, c.updated_at,
			%s as participants,
			%s as tags,
			%s as snippet
		from conversations c
		where c.id = ? or c.provider_id = ?
		order by c.updated_at desc
		limit 1`, participantsSelect, tagsSelect, snippetSelect), id, id).Scan(
		&detail.ID,
		&detail.ProviderID,
		&detail.Subject,
		&detail.State,
		&detail.Assignee,
		&detail.Rating,
		&detail.FinStatus,
		&detail.CreatedAt,
		&detail.UpdatedAt,
		&participants,
		&tags,
		&snippet,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return ConversationDetail{}, fmt.Errorf("conversation %q not found", id)
		}
		return ConversationDetail{}, err
	}
	detail.Participants = splitList(participants)
	detail.Tags = splitList(tags)
	detail.Snippet = SafeSnippet(snippet, 240)
	return detail, nil
}

func getConversationParts(ctx context.Context, db *sql.DB, conversationID string, limit int) ([]ConversationPartDetail, error) {
	hasPartsTable, err := tableExists(ctx, db, "conversation_parts")
	if err != nil {
		return nil, err
	}
	if !hasPartsTable {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `select id, provider_id, part_type, author_name, body, created_at, updated_at
		from conversation_parts
		where conversation_id = ?
		order by created_at, provider_id
		limit ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var parts []ConversationPartDetail
	for rows.Next() {
		var part ConversationPartDetail
		if err := rows.Scan(&part.ID, &part.ProviderID, &part.Type, &part.AuthorName, &part.Body, &part.CreatedAt, &part.UpdatedAt); err != nil {
			return nil, err
		}
		part.Body = SafeSnippet(part.Body, 1200)
		parts = append(parts, part)
	}
	return parts, rows.Err()
}
