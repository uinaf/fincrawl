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
	Score        float64  `json:"score,omitempty"`
}

type SearchOptions struct {
	Limit     int
	State     string
	FinStatus string
	Tag       string
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
	return SearchWithOptions(ctx, dbPath, query, SearchOptions{Limit: limit})
}

func SearchWithOptions(ctx context.Context, dbPath, query string, opts SearchOptions) ([]SearchResult, error) {
	opts = normalizeSearchOptions(opts)
	if SanitizeFTSQuery(query) == "" {
		return nil, fmt.Errorf("empty search query")
	}
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	results, err := searchFTS(ctx, st.DB(), query, opts)
	if err == nil && len(results) > 0 {
		return results, nil
	}
	return searchLike(ctx, st.DB(), query, opts)
}

func normalizeSearchOptions(opts SearchOptions) SearchOptions {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	opts.State = strings.TrimSpace(opts.State)
	opts.FinStatus = strings.TrimSpace(opts.FinStatus)
	opts.Tag = strings.TrimSpace(opts.Tag)
	return opts
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

func searchFTS(ctx context.Context, db *sql.DB, query string, opts SearchOptions) ([]SearchResult, error) {
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
	filters, args := searchFilters(opts)
	args = append([]any{ftsQuery}, args...)
	args = append(args, opts.Limit)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`select c.id, c.provider_id, c.subject, c.state, c.assignee, c.rating, c.fin_status, c.updated_at,
		%s as participants,
		coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '') as tags,
			snippet(conversation_fts, 2, '', '', ' ... ', 24) as snippet,
			(-bm25(conversation_fts, 0.0, 10.0, 5.0, 3.0, 2.0, 1.5, 1.0, 1.0, 1.0)) as score
	from conversation_fts
	join conversations c on c.id = conversation_fts.conversation_id
	where conversation_fts match ?
	%s
		order by bm25(conversation_fts, 0.0, 10.0, 5.0, 3.0, 2.0, 1.5, 1.0, 1.0, 1.0), c.updated_at desc
	limit ?`, participantsSelect, filters), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func searchLike(ctx context.Context, db *sql.DB, query string, opts SearchOptions) ([]SearchResult, error) {
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
	hasFTSTable, err := tableExists(ctx, db, "conversation_fts")
	if err != nil {
		return nil, err
	}
	participantScore := `exists(select 1 from conversation_participants cp where cp.conversation_id = c.id and lower(cp.name) like ?)`
	hasParticipantSearch := true
	if hasParticipantTable && !hasFTSTable {
		participantsSelect = `coalesce(nullif((select group_concat(cp.name, ', ') from conversation_participants cp where cp.conversation_id = c.id), ''), '')`
	} else if !hasParticipantTable && hasFTSTable {
		participantsSelect = `coalesce((select f.participants from conversation_fts f where f.conversation_id = c.id), '')`
		participantScore = `exists(select 1 from conversation_fts f where f.conversation_id = c.id and lower(f.participants) like ?)`
		participantPredicate = `or exists(select 1 from conversation_fts f where f.conversation_id = c.id and lower(f.participants) like ?)`
	} else if !hasParticipantTable {
		participantsSelect = `''`
		participantScore = `0`
		participantPredicate = ``
		hasParticipantSearch = false
	}
	scoreArgs := []any{like, like, like}
	whereArgs := []any{like, like, like}
	if hasParticipantSearch {
		scoreArgs = append(scoreArgs, like)
		whereArgs = append(whereArgs, like)
	}
	scoreArgs = append(scoreArgs, like, like, like)
	whereArgs = append(whereArgs, like, like, like)
	filters, filterArgs := searchFilters(opts)
	args := append(scoreArgs, whereArgs...)
	args = append(args, filterArgs...)
	args = append(args, opts.Limit)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`select c.id, c.provider_id, c.subject, c.state, c.assignee, c.rating, c.fin_status, c.updated_at,
		%s as participants,
		coalesce((select group_concat(t.name, ', ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), '') as tags,
		substr(coalesce((select group_concat(p.body, ' ') from conversation_parts p where p.conversation_id = c.id), c.subject), 1, 240) as snippet,
		(
				case when lower(c.subject) like ? then 100.0 else 0.0 end +
				case when exists(select 1 from conversation_parts p where p.conversation_id = c.id and lower(p.body) like ?) then 50.0 else 0.0 end +
				case when exists(select 1 from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id and lower(t.name) like ?) then 20.0 else 0.0 end +
				case when %s then 15.0 else 0.0 end +
				case when lower(c.assignee) like ? then 10.0 else 0.0 end +
				case when lower(c.rating) like ? then 5.0 else 0.0 end +
				case when lower(c.fin_status) like ? then 5.0 else 0.0 end
		) as score
	from conversations c
	where (
		lower(c.subject) like ?
		or exists(select 1 from conversation_parts p where p.conversation_id = c.id and lower(p.body) like ?)
		or exists(select 1 from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id and lower(t.name) like ?)
		%s
		or lower(c.assignee) like ?
		or lower(c.rating) like ?
		or lower(c.fin_status) like ?
		)
			%s
		order by score desc, c.updated_at desc
		limit ?`, participantsSelect, participantScore, participantPredicate, filters), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func searchFilters(opts SearchOptions) (string, []any) {
	var clauses []string
	var args []any
	if opts.State != "" {
		clauses = append(clauses, "and lower(c.state) = lower(?)")
		args = append(args, opts.State)
	}
	if opts.FinStatus != "" {
		clauses = append(clauses, "and lower(c.fin_status) = lower(?)")
		args = append(args, opts.FinStatus)
	}
	if opts.Tag != "" {
		clauses = append(clauses, `and exists(
			select 1 from conversation_tags ct
			join tags t on t.id = ct.tag_id
			where ct.conversation_id = c.id and lower(t.name) = lower(?)
		)`)
		args = append(args, opts.Tag)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "\n\t" + strings.Join(clauses, "\n\t"), args
}

func scanResults(rows *sql.Rows) ([]SearchResult, error) {
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var tags string
		var participants string
		if err := rows.Scan(&result.ID, &result.ProviderID, &result.Subject, &result.State, &result.Assignee, &result.Rating, &result.FinStatus, &result.UpdatedAt, &participants, &tags, &result.Snippet, &result.Score); err != nil {
			return nil, err
		}
		result.Snippet = SafeSnippet(result.Snippet, 240)
		result.Participants = splitList(participants)
		result.Tags = splitList(tags)
		results = append(results, result)
	}
	return results, rows.Err()
}

func SafeSnippet(value string, limit int) string {
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			if builder.Len() > 0 {
				lastSpace = true
			}
			continue
		}
		if lastSpace {
			builder.WriteByte(' ')
			lastSpace = false
		}
		builder.WriteRune(r)
	}
	snippet := strings.TrimSpace(builder.String())
	if limit <= 0 {
		return snippet
	}
	runes := []rune(snippet)
	if len(runes) <= limit {
		return snippet
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
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
