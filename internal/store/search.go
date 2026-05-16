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
	ID         string   `json:"id"`
	ProviderID string   `json:"provider_id"`
	Subject    string   `json:"subject"`
	State      string   `json:"state"`
	Assignee   string   `json:"assignee,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	UpdatedAt  string   `json:"updated_at"`
	Snippet    string   `json:"snippet"`
}

type CountsResult struct {
	Conversations     int64
	ConversationParts int64
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
	if err == nil {
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
	if err := st.DB().QueryRowContext(ctx, `select count(*) from conversations`).Scan(&counts.Conversations); err != nil {
		return CountsResult{}, err
	}
	if err := st.DB().QueryRowContext(ctx, `select count(*) from conversation_parts`).Scan(&counts.ConversationParts); err != nil {
		return CountsResult{}, err
	}
	if err := st.DB().QueryRowContext(ctx, `select count(*) from raw_blobs`).Scan(&counts.RawBlobs); err != nil {
		return CountsResult{}, err
	}
	return counts, nil
}

func searchFTS(ctx context.Context, db *sql.DB, query string, limit int) ([]SearchResult, error) {
	ftsQuery := SanitizeFTSQuery(query)
	if ftsQuery == "" {
		return nil, fmt.Errorf("empty search query")
	}
	rows, err := db.QueryContext(ctx, `select c.id, c.provider_id, c.subject, c.state, c.assignee, c.updated_at,
		coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '') as tags,
		snippet(conversation_fts, 2, '', '', ' ... ', 24) as snippet
	from conversation_fts
	join conversations c on c.id = conversation_fts.conversation_id
	where conversation_fts match ?
	order by c.updated_at desc
	limit ?`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func searchLike(ctx context.Context, db *sql.DB, query string, limit int) ([]SearchResult, error) {
	like := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := db.QueryContext(ctx, `select c.id, c.provider_id, c.subject, c.state, c.assignee, c.updated_at,
		coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '') as tags,
		substr(coalesce((select group_concat(p.body, ' ') from conversation_parts p where p.conversation_id = c.id), c.subject), 1, 240) as snippet
	from conversations c
	where lower(c.subject) like ?
		or exists(select 1 from conversation_parts p where p.conversation_id = c.id and lower(p.body) like ?)
		or exists(select 1 from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id and lower(t.name) like ?)
		or lower(c.assignee) like ?
	order by c.updated_at desc
	limit ?`, like, like, like, like, limit)
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
		if err := rows.Scan(&result.ID, &result.ProviderID, &result.Subject, &result.State, &result.Assignee, &result.UpdatedAt, &tags, &result.Snippet); err != nil {
			return nil, err
		}
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				result.Tags = append(result.Tags, tag)
			}
		}
		results = append(results, result)
	}
	return results, rows.Err()
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
