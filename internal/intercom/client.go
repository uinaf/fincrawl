package intercom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultAPIVersion = "2.15"

type Client struct {
	BaseURL       string
	Token         string
	Version       string
	HTTPClient    *http.Client
	Sleep         func(context.Context, time.Duration) error
	Now           func() time.Time
	ThrottleBelow int
	MaxAttempts   int
	RetryBackoff  time.Duration
}

type ConversationListItem struct {
	ID        string `json:"id"`
	UpdatedAt int64  `json:"updated_at"`
}

type ConversationSearchResult struct {
	Conversations []ConversationListItem
	NextCursor    string
}

type Conversation struct {
	ID                string          `json:"id"`
	ConversationParts json.RawMessage `json:"conversation_parts"`
	Raw               json.RawMessage `json:"-"`
}

type Entity struct {
	ID      string
	Name    string
	Email   string
	TeamIDs []string
	Raw     map[string]any
}

type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("intercom rate limited: retry after %s", e.RetryAfter)
	}
	return "intercom rate limited"
}

type HTTPStatusError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e HTTPStatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("intercom %s %s failed: status %d", e.Method, e.Path, e.StatusCode)
	}
	return fmt.Sprintf("intercom %s %s failed: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func (c Client) SearchConversations(ctx context.Context, updatedAfter, updatedBefore time.Time, startingAfter string) (ConversationSearchResult, error) {
	body := searchBody(updatedAfter, updatedBefore, startingAfter)
	var response struct {
		Conversations []ConversationListItem `json:"conversations"`
		Pages         struct {
			Next struct {
				StartingAfter string `json:"starting_after"`
			} `json:"next"`
		} `json:"pages"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/conversations/search", nil, body, &response); err != nil {
		return ConversationSearchResult{}, err
	}
	return ConversationSearchResult{Conversations: response.Conversations, NextCursor: response.Pages.Next.StartingAfter}, nil
}

func (c Client) RetrieveConversation(ctx context.Context, id string) (Conversation, error) {
	query := url.Values{"display_as": []string{"plaintext"}}
	var conversation Conversation
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/conversations/"+url.PathEscape(id), query, nil, &raw); err != nil {
		return Conversation{}, err
	}
	if err := json.Unmarshal(raw, &conversation); err != nil {
		return Conversation{}, fmt.Errorf("decode conversation: %w", err)
	}
	conversation.Raw = raw
	return conversation, nil
}

func (c Client) ListAdmins(ctx context.Context) ([]Entity, error) {
	return c.listEntities(ctx, "/admins", nil, "admins", "data")
}

func (c Client) ListTeams(ctx context.Context) ([]Entity, error) {
	return c.listEntities(ctx, "/teams", nil, "teams", "data")
}

func (c Client) ListTags(ctx context.Context) ([]Entity, error) {
	return c.listEntities(ctx, "/tags", nil, "tags", "data")
}

func (c Client) ListContacts(ctx context.Context, limit int) ([]Entity, error) {
	if limit < 0 {
		return nil, fmt.Errorf("contact limit must be >= 0")
	}
	if limit == 0 {
		limit = 50
	}
	var entities []Entity
	var cursor string
	for len(entities) < limit {
		remaining := limit - len(entities)
		perPage := remaining
		if perPage > 150 {
			perPage = 150
		}
		query := url.Values{"per_page": []string{strconv.Itoa(perPage)}}
		if cursor != "" {
			query.Set("starting_after", cursor)
		}
		var raw json.RawMessage
		if err := c.doJSON(ctx, http.MethodGet, "/contacts", query, nil, &raw); err != nil {
			return nil, err
		}
		pageEntities, next, err := decodeEntitiesPage(raw, "data", "contacts")
		if err != nil {
			return nil, err
		}
		entities = append(entities, pageEntities...)
		if next == "" || len(pageEntities) == 0 {
			break
		}
		cursor = next
	}
	if len(entities) > limit {
		entities = entities[:limit]
	}
	return entities, nil
}

func (c Client) listEntities(ctx context.Context, path string, query url.Values, keys ...string) ([]Entity, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, path, query, nil, &raw); err != nil {
		return nil, err
	}
	entities, _, err := decodeEntitiesPage(raw, keys...)
	return entities, err
}

func (c Client) doJSON(ctx context.Context, method, path string, query url.Values, requestBody any, responseBody any) error {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.intercom.io"
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	var bodyData []byte
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		bodyData = data
	}
	version := c.Version
	if version == "" {
		version = DefaultAPIVersion
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	attempts := c.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		var body io.Reader
		if bodyData != nil {
			body = bytes.NewReader(bodyData)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Intercom-Version", version)
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := client.Do(req)
		if err != nil {
			if attempt < attempts && shouldRetryError(err) {
				if sleepErr := c.sleep(ctx, retryDelay(c.RetryBackoff, attempt)); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			delay := retryAfter(resp.Header, c.now())
			resp.Body.Close()
			return RateLimitError{StatusCode: resp.StatusCode, RetryAfter: delay}
		}
		if resp.StatusCode >= 500 && resp.StatusCode <= 599 && attempt < attempts {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if sleepErr := c.sleep(ctx, retryDelay(c.RetryBackoff, attempt)); sleepErr != nil {
				return sleepErr
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return HTTPStatusError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
		}
		if err := c.maybeThrottle(ctx, resp.Header); err != nil {
			resp.Body.Close()
			return err
		}
		err = json.NewDecoder(resp.Body).Decode(responseBody)
		resp.Body.Close()
		return err
	}
	return nil
}

func shouldRetryError(err error) bool {
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if attempt <= 1 {
		return base
	}
	return time.Duration(attempt) * base
}

func decodeEntitiesPage(raw json.RawMessage, keys ...string) ([]Entity, string, error) {
	items, err := rawItems(raw, keys...)
	if err != nil {
		return nil, "", err
	}
	entities := make([]Entity, 0, len(items))
	for _, item := range items {
		var value map[string]any
		if err := json.Unmarshal(item, &value); err != nil {
			return nil, "", fmt.Errorf("decode entity: %w", err)
		}
		id := stringValue(value, "id")
		if id == "" {
			continue
		}
		entities = append(entities, Entity{
			ID:      id,
			Name:    firstNonEmpty(stringValue(value, "name"), stringValue(value, "email"), id),
			Email:   stringValue(value, "email"),
			TeamIDs: entityTeamIDs(value),
			Raw:     value,
		})
	}
	return entities, nextCursor(raw), nil
}

func rawItems(raw json.RawMessage, keys ...string) ([]json.RawMessage, error) {
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		return array, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode entity page: %w", err)
	}
	for _, key := range keys {
		if body, ok := root[key]; ok {
			var values []json.RawMessage
			if err := json.Unmarshal(body, &values); err != nil {
				return nil, fmt.Errorf("decode %s list: %w", key, err)
			}
			return values, nil
		}
	}
	return nil, fmt.Errorf("entity list missing expected keys %s", strings.Join(keys, ", "))
}

func nextCursor(raw json.RawMessage) string {
	var root struct {
		Pages struct {
			Next struct {
				StartingAfter string `json:"starting_after"`
			} `json:"next"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return ""
	}
	return strings.TrimSpace(root.Pages.Next.StartingAfter)
}

func entityTeamIDs(value map[string]any) []string {
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" {
			seen[id] = true
		}
	}
	for _, value := range arrayAny(value["team_ids"]) {
		add(fmt.Sprint(value))
	}
	if teams, ok := value["teams"].(map[string]any); ok {
		for _, value := range arrayAny(teams["teams"]) {
			if team, ok := value.(map[string]any); ok {
				add(stringValue(team, "id"))
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func arrayAny(value any) []any {
	array, _ := value.([]any)
	return array
}

func stringValue(raw map[string]any, key string) string {
	switch value := raw[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		if value == float64(int64(value)) {
			return fmt.Sprintf("%d", int64(value))
		}
		return fmt.Sprint(value)
	default:
		return ""
	}
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

func (c Client) maybeThrottle(ctx context.Context, header http.Header) error {
	if c.Sleep == nil {
		return nil
	}
	threshold := c.ThrottleBelow
	if threshold <= 0 {
		threshold = 10
	}
	remaining, err := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	if err != nil || remaining >= threshold {
		return nil
	}
	delay := resetDelay(header, c.now())
	if delay <= 0 {
		delay = 1500 * time.Millisecond
	}
	return c.Sleep(ctx, delay)
}

func (c Client) sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if c.Sleep != nil {
		return c.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func retryAfter(header http.Header, now time.Time) time.Duration {
	if value := header.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return resetDelay(header, now)
}

func resetDelay(header http.Header, now time.Time) time.Duration {
	value := header.Get("X-RateLimit-Reset")
	if value == "" {
		return 0
	}
	resetUnix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	delay := time.Unix(resetUnix, 0).Sub(now)
	if delay < 0 {
		return 0
	}
	return delay
}

func searchBody(updatedAfter, updatedBefore time.Time, startingAfter string) map[string]any {
	after, before := inclusiveSecondBounds(updatedAfter, updatedBefore)
	body := map[string]any{
		"query": map[string]any{
			"operator": "AND",
			"value": []map[string]any{
				{"field": "updated_at", "operator": ">", "value": after.Unix()},
				{"field": "updated_at", "operator": "<", "value": before.Unix()},
			},
		},
		"sort": map[string]any{
			"field": "updated_at",
			"order": "ascending",
		},
		"pagination": map[string]any{
			"per_page": 150,
		},
	}
	if strings.TrimSpace(startingAfter) != "" {
		body["pagination"].(map[string]any)["starting_after"] = startingAfter
	}
	return body
}

func inclusiveSecondBounds(updatedAfter, updatedBefore time.Time) (time.Time, time.Time) {
	return updatedAfter.Add(-time.Second), updatedBefore.Add(time.Second)
}
