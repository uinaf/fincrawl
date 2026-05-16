package intercom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	version := c.Version
	if version == "" {
		version = DefaultAPIVersion
	}
	req.Header.Set("Intercom-Version", version)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return RateLimitError{StatusCode: resp.StatusCode, RetryAfter: retryAfter(resp.Header, c.now())}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("intercom %s %s failed: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := c.maybeThrottle(ctx, resp.Header); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(responseBody)
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
	body := map[string]any{
		"query": map[string]any{
			"operator": "AND",
			"value": []map[string]any{
				{"field": "updated_at", "operator": ">", "value": updatedAfter.Unix()},
				{"field": "updated_at", "operator": "<", "value": updatedBefore.Unix()},
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
