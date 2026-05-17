package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"

	ckstore "github.com/openclaw/crawlkit/store"
)

type SearchResult struct {
	ID           string   `json:"id"`
	ProviderID   string   `json:"provider_id"`
	Subject      string   `json:"subject"`
	State        string   `json:"state"`
	Assignee     string   `json:"assignee,omitempty"`
	Rating       string   `json:"rating,omitempty"`
	FinStatus    string   `json:"fin_status,omitempty"`
	Participants []string `json:"participants,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	UpdatedAt    string   `json:"updated_at"`
	Snippet      string   `json:"snippet"`
}

type CountsResult struct {
	Conversations     int64
	ConversationParts int64
	Admins            int64
	Teams             int64
	Tags              int64
	Contacts          int64
	RawBlobs          int64
}

func Search(ctx context.Context, dbPath, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if SanitizeFTSQuery(query) == "" {
		return nil, fmt.Errorf("empty search query")
	}
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	results, err := searchFTS(ctx, st.DB(), query, limit)
	if err == nil && len(results) > 0 {
		return results, nil
	}
	return searchLike(ctx, st.DB(), query, limit)
}

func Counts(ctx context.Context, dbPath string) (CountsResult, error) {
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return CountsResult{}, err
	}
	defer st.Close()
	var counts CountsResult
	if counts.Conversations, err = countRows(ctx, st.DB(), "conversations"); err != nil {
		return CountsResult{}, err
	}
	if counts.ConversationParts, err = countRows(ctx, st.DB(), "conversation_parts"); err != nil {
		return CountsResult{}, err
	}
	if counts.Admins, err = countRows(ctx, st.DB(), "admins"); err != nil {
		return CountsResult{}, err
	}
	if counts.Teams, err = countRows(ctx, st.DB(), "teams"); err != nil {
		return CountsResult{}, err
	}
	if counts.Tags, err = countRows(ctx, st.DB(), "provider_tags"); err != nil {
		return CountsResult{}, err
	}
	if counts.Contacts, err = countRows(ctx, st.DB(), "contacts"); err != nil {
		return CountsResult{}, err
	}
	if counts.RawBlobs, err = countRows(ctx, st.DB(), "raw_blobs"); err != nil {
		return CountsResult{}, err
	}
	return counts, nil
}

func countRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	exists, err := tableExists(ctx, db, table)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var count int64
	if err := db.QueryRowContext(ctx, `select count(*) from `+ckstore.QuoteIdent(table)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `select count(*) from sqlite_master where type = 'table' and name = ?`, table).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func searchFTS(ctx context.Context, db *sql.DB, query string, limit int) ([]SearchResult, error) {
	ftsQuery := SanitizeFTSQuery(query)
	if ftsQuery == "" {
		return nil, fmt.Errorf("empty search query")
	}
	participantsSelect := `coalesce(
		nullif((select group_concat(cp.name, ', ') from conversation_participants cp where cp.conversation_id = c.id), ''),
		conversation_fts.participants,
		''
	)`
	hasParticipantTable, err := tableExists(ctx, db, "conversation_participants")
	if err != nil {
		return nil, err
	}
	if !hasParticipantTable {
		participantsSelect = `conversation_fts.participants`
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`select c.id, c.provider_id, c.subject, c.state, c.assignee, c.rating, c.fin_status, c.updated_at,
		%s as participants,
		coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '') as tags,
		snippet(conversation_fts, 2, '', '', ' ... ', 24) as snippet
	from conversation_fts
	join conversations c on c.id = conversation_fts.conversation_id
	where conversation_fts match ?
	order by c.updated_at desc
	limit ?`, participantsSelect), ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func searchLike(ctx context.Context, db *sql.DB, query string, limit int) ([]SearchResult, error) {
	like := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	participantsSelect := `coalesce(
		nullif((select group_concat(cp.name, ', ') from conversation_participants cp where cp.conversation_id = c.id), ''),
		(select f.participants from conversation_fts f where f.conversation_id = c.id),
		''
	)`
	participantPredicate := `or exists(select 1 from conversation_participants cp where cp.conversation_id = c.id and lower(cp.name) like ?)`
	hasParticipantTable, err := tableExists(ctx, db, "conversation_participants")
	if err != nil {
		return nil, err
	}
	args := []any{like, like, like}
	if !hasParticipantTable {
		participantsSelect = `coalesce((select f.participants from conversation_fts f where f.conversation_id = c.id), '')`
		participantPredicate = `or exists(select 1 from conversation_fts f where f.conversation_id = c.id and lower(f.participants) like ?)`
	}
	args = append(args, like, like, like, like, limit)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`select c.id, c.provider_id, c.subject, c.state, c.assignee, c.rating, c.fin_status, c.updated_at,
		%s as participants,
		coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '') as tags,
		substr(coalesce((select group_concat(p.body, ' ') from conversation_parts p where p.conversation_id = c.id), c.subject), 1, 240) as snippet
	from conversations c
	where lower(c.subject) like ?
		or exists(select 1 from conversation_parts p where p.conversation_id = c.id and lower(p.body) like ?)
		or exists(select 1 from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id and lower(t.name) like ?)
		%s
		or lower(c.assignee) like ?
		or lower(c.rating) like ?
		or lower(c.fin_status) like ?
	order by c.updated_at desc
	limit ?`, participantsSelect, participantPredicate), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func scanResults(rows *sql.Rows) ([]SearchResult, error) {
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var tags string
		var participants string
		if err := rows.Scan(&result.ID, &result.ProviderID, &result.Subject, &result.State, &result.Assignee, &result.Rating, &result.FinStatus, &result.UpdatedAt, &participants, &tags, &result.Snippet); err != nil {
			return nil, err
		}
		result.Participants = splitList(participants)
		result.Tags = splitList(tags)
		results = append(results, result)
	}
	return results, rows.Err()
}

func splitList(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

func SanitizeFTSQuery(query string) string {
	var terms []string
	for _, field := range strings.Fields(normalizeSearchText(query)) {
		if field != "" && !isFTSOperator(field) {
			terms = append(terms, field)
		}
	}
	return strings.Join(terms, " ")
}

func normalizeSearchText(query string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, query)
}

func isFTSOperator(value string) bool {
	switch strings.ToUpper(value) {
	case "AND", "OR", "NOT", "NEAR":
		return true
	default:
		return false
	}
}
