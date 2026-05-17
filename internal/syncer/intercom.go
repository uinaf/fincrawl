package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/uinaf/fincrawl/internal/intercom"
	"github.com/uinaf/fincrawl/internal/store"
)

const defaultWorkspaceID = "intercom"
const maxRateLimitRetries = 3

type IntercomSyncer struct {
	Client    intercom.Client
	Workspace store.Workspace
	Now       func() time.Time
}

type TailSyncOptions struct {
	UpdatedAfter  time.Time
	UpdatedBefore time.Time
	Limit         int
	Resume        bool
}

type EntitySyncOptions struct {
	IncludeContacts bool
	ContactLimit    int
}

func (s IntercomSyncer) SyncConversation(ctx context.Context, dbPath, conversationID string) (store.SyncResult, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return store.SyncResult{}, fmt.Errorf("conversation id is required")
	}
	conversation, err := s.retrieveConversation(ctx, conversationID)
	if err != nil {
		return store.SyncResult{}, err
	}
	normalized, err := NormalizeConversation(conversation)
	if err != nil {
		return store.SyncResult{}, err
	}
	return store.SyncConversations(ctx, dbPath, s.workspace(), []store.Conversation{normalized})
}

func (s IntercomSyncer) SyncUpdatedSince(ctx context.Context, dbPath string, updatedAfter, updatedBefore time.Time, limit int) (store.SyncResult, error) {
	return s.SyncTail(ctx, dbPath, TailSyncOptions{UpdatedAfter: updatedAfter, UpdatedBefore: updatedBefore, Limit: limit})
}

func (s IntercomSyncer) ResumeTail(ctx context.Context, dbPath string, limit int) (store.SyncResult, error) {
	return s.SyncTail(ctx, dbPath, TailSyncOptions{Limit: limit, Resume: true})
}

func (s IntercomSyncer) SyncEntities(ctx context.Context, dbPath string, opts EntitySyncOptions) (store.SyncResult, error) {
	var entities store.Entities
	var warnings []string
	succeeded := false
	admins, err := s.listAdmins(ctx)
	if err != nil {
		if isScopeDenied(err) {
			warnings = append(warnings, "admins unavailable: Intercom scope denied")
		} else {
			return store.SyncResult{}, err
		}
	} else {
		succeeded = true
		entities.Admins = normalizeAdmins(admins)
	}
	teams, err := s.listTeams(ctx)
	if err != nil {
		if isScopeDenied(err) {
			warnings = append(warnings, "teams unavailable: Intercom scope denied")
		} else {
			return store.SyncResult{}, err
		}
	} else {
		succeeded = true
		entities.Teams = normalizeTeams(teams)
	}
	tags, err := s.listTags(ctx)
	if err != nil {
		if isScopeDenied(err) {
			warnings = append(warnings, "tags unavailable: Intercom scope denied")
		} else {
			return store.SyncResult{}, err
		}
	} else {
		succeeded = true
		entities.Tags = normalizeProviderTags(tags)
	}
	if opts.IncludeContacts {
		limit := opts.ContactLimit
		if limit <= 0 {
			limit = 50
		}
		contacts, err := s.listContacts(ctx, limit)
		if err != nil {
			if isScopeDenied(err) {
				warnings = append(warnings, "contacts unavailable: Intercom scope denied")
			} else {
				return store.SyncResult{}, err
			}
		} else {
			succeeded = true
			entities.Contacts = normalizeContacts(contacts)
		}
	}
	result, err := store.SyncEntities(ctx, dbPath, s.workspace(), entities)
	if err != nil {
		return store.SyncResult{}, err
	}
	result.Warnings = warnings
	if !succeeded && len(warnings) > 0 {
		return store.SyncResult{}, fmt.Errorf("no Intercom entity scopes available; credential may be invalid or missing read scopes")
	}
	return result, nil
}

func (s IntercomSyncer) SyncTail(ctx context.Context, dbPath string, opts TailSyncOptions) (store.SyncResult, error) {
	state, cursor, skipUntil, err := s.prepareTailState(ctx, dbPath, opts)
	if err != nil {
		return store.SyncResult{}, err
	}
	updatedAfter, updatedBefore, err := stateWindow(state)
	if err != nil {
		return store.SyncResult{}, err
	}
	var result store.SyncResult
	result.WorkspaceID = s.workspace().ID
	importedAny := false
	for {
		state.PageCursor = cursor
		if err := store.SaveSyncState(ctx, dbPath, state); err != nil {
			return store.SyncResult{}, err
		}
		page, err := s.searchConversations(ctx, updatedAfter, updatedBefore, cursor)
		if err != nil {
			return store.SyncResult{}, err
		}
		for _, item := range page.Conversations {
			if skipUntil != "" {
				if item.ID == skipUntil {
					skipUntil = ""
				}
				continue
			}
			conversation, err := s.retrieveConversation(ctx, item.ID)
			if err != nil {
				return store.SyncResult{}, err
			}
			normalized, err := NormalizeConversation(conversation)
			if err != nil {
				return store.SyncResult{}, err
			}
			importedAny = true
			imported, err := store.SyncConversations(ctx, dbPath, s.workspace(), []store.Conversation{normalized})
			if err != nil {
				return store.SyncResult{}, err
			}
			result.Conversations += imported.Conversations
			result.ConversationParts += imported.ConversationParts
			result.RawBlobs += imported.RawBlobs
			state.LastProviderID = item.ID
			if err := store.SaveSyncState(ctx, dbPath, state); err != nil {
				return store.SyncResult{}, err
			}
			if opts.Limit > 0 && result.Conversations >= opts.Limit {
				return result, nil
			}
		}
		if skipUntil != "" {
			return store.SyncResult{}, fmt.Errorf("resume marker %q was not found in provider page; leaving active sync state unchanged", skipUntil)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		state.PageCursor = cursor
		state.LastProviderID = ""
	}
	if !importedAny {
		empty, err := store.SyncConversations(ctx, dbPath, s.workspace(), nil)
		if err != nil {
			return store.SyncResult{}, err
		}
		result.WorkspaceID = empty.WorkspaceID
	}
	state.HighWaterMark = advanceHighWaterMark(state.HighWaterMark, state.ActiveWindowEnd)
	state.ActiveWindowStart = ""
	state.ActiveWindowEnd = ""
	state.PageCursor = ""
	state.LastProviderID = ""
	if err := store.SaveSyncState(ctx, dbPath, state); err != nil {
		return store.SyncResult{}, err
	}
	return result, nil
}

func (s IntercomSyncer) prepareTailState(ctx context.Context, dbPath string, opts TailSyncOptions) (store.SyncState, string, string, error) {
	if opts.Limit < 0 {
		return store.SyncState{}, "", "", fmt.Errorf("limit must be >= 0")
	}
	if opts.Resume {
		state, ok, err := store.LoadSyncState(ctx, dbPath, store.IntercomTailSyncStateID)
		if err != nil {
			return store.SyncState{}, "", "", err
		}
		if !ok || state.ActiveWindowStart == "" || state.ActiveWindowEnd == "" {
			return store.SyncState{}, "", "", fmt.Errorf("no active Intercom tail sync state to resume")
		}
		return state, state.PageCursor, state.LastProviderID, nil
	}
	if opts.UpdatedAfter.IsZero() || opts.UpdatedBefore.IsZero() {
		return store.SyncState{}, "", "", fmt.Errorf("updated tail sync window is required")
	}
	previous, _, err := store.LoadSyncState(ctx, dbPath, store.IntercomTailSyncStateID)
	if err != nil {
		return store.SyncState{}, "", "", err
	}
	if previous.ActiveWindowStart != "" || previous.ActiveWindowEnd != "" {
		return store.SyncState{}, "", "", fmt.Errorf("active Intercom tail sync state exists; run sync --resume before starting a new window")
	}
	state := store.SyncState{
		ID:                store.IntercomTailSyncStateID,
		Provider:          store.ProviderIntercom,
		CursorKind:        "updated_at",
		HighWaterMark:     previous.HighWaterMark,
		ActiveWindowStart: opts.UpdatedAfter.UTC().Format(time.RFC3339),
		ActiveWindowEnd:   opts.UpdatedBefore.UTC().Format(time.RFC3339),
	}
	return state, "", "", nil
}

func stateWindow(state store.SyncState) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, state.ActiveWindowStart)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse active sync window start: %w", err)
	}
	end, err := time.Parse(time.RFC3339, state.ActiveWindowEnd)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse active sync window end: %w", err)
	}
	return start, end, nil
}

func advanceHighWaterMark(current, candidate string) string {
	if strings.TrimSpace(candidate) == "" {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return candidate
	}
	currentTime, currentErr := time.Parse(time.RFC3339, current)
	candidateTime, candidateErr := time.Parse(time.RFC3339, candidate)
	if candidateErr != nil && currentErr == nil {
		return current
	}
	if candidateErr != nil || currentErr != nil {
		return candidate
	}
	if candidateTime.After(currentTime) {
		return candidate
	}
	return current
}

func (s IntercomSyncer) searchConversations(ctx context.Context, updatedAfter, updatedBefore time.Time, cursor string) (intercom.ConversationSearchResult, error) {
	var result intercom.ConversationSearchResult
	err := s.withRateLimitRetry(ctx, func() error {
		var err error
		result, err = s.Client.SearchConversations(ctx, updatedAfter, updatedBefore, cursor)
		return err
	})
	return result, err
}

func (s IntercomSyncer) retrieveConversation(ctx context.Context, id string) (intercom.Conversation, error) {
	var result intercom.Conversation
	err := s.withRateLimitRetry(ctx, func() error {
		var err error
		result, err = s.Client.RetrieveConversation(ctx, id)
		return err
	})
	return result, err
}

func (s IntercomSyncer) listAdmins(ctx context.Context) ([]intercom.Entity, error) {
	var result []intercom.Entity
	err := s.withRateLimitRetry(ctx, func() error {
		var err error
		result, err = s.Client.ListAdmins(ctx)
		return err
	})
	return result, err
}

func (s IntercomSyncer) listTeams(ctx context.Context) ([]intercom.Entity, error) {
	var result []intercom.Entity
	err := s.withRateLimitRetry(ctx, func() error {
		var err error
		result, err = s.Client.ListTeams(ctx)
		return err
	})
	return result, err
}

func (s IntercomSyncer) listTags(ctx context.Context) ([]intercom.Entity, error) {
	var result []intercom.Entity
	err := s.withRateLimitRetry(ctx, func() error {
		var err error
		result, err = s.Client.ListTags(ctx)
		return err
	})
	return result, err
}

func (s IntercomSyncer) listContacts(ctx context.Context, limit int) ([]intercom.Entity, error) {
	var result []intercom.Entity
	err := s.withRateLimitRetry(ctx, func() error {
		var err error
		result, err = s.Client.ListContacts(ctx, limit)
		return err
	})
	return result, err
}

func (s IntercomSyncer) withRateLimitRetry(ctx context.Context, call func() error) error {
	for attempt := 0; ; attempt++ {
		err := call()
		if err == nil {
			return nil
		}
		var rateErr intercom.RateLimitError
		if !errors.As(err, &rateErr) || attempt >= maxRateLimitRetries {
			return err
		}
		delay := rateErr.RetryAfter
		if delay <= 0 {
			delay = time.Second
		}
		if err := s.sleep(ctx, delay); err != nil {
			return err
		}
	}
}

func isScopeDenied(err error) bool {
	var statusErr intercom.HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden
}

func normalizeAdmins(entities []intercom.Entity) []store.Admin {
	admins := make([]store.Admin, 0, len(entities))
	for _, entity := range entities {
		admins = append(admins, store.Admin{
			ID:         localID("admin", entity.ID),
			Provider:   store.ProviderIntercom,
			ProviderID: entity.ID,
			Name:       entity.Name,
			Email:      entity.Email,
			TeamIDs:    entity.TeamIDs,
			Raw:        entity.Raw,
		})
	}
	return admins
}

func normalizeTeams(entities []intercom.Entity) []store.Team {
	teams := make([]store.Team, 0, len(entities))
	for _, entity := range entities {
		teams = append(teams, store.Team{
			ID:         localID("team", entity.ID),
			Provider:   store.ProviderIntercom,
			ProviderID: entity.ID,
			Name:       entity.Name,
			Raw:        entity.Raw,
		})
	}
	return teams
}

func normalizeProviderTags(entities []intercom.Entity) []store.ProviderTag {
	tags := make([]store.ProviderTag, 0, len(entities))
	for _, entity := range entities {
		tags = append(tags, store.ProviderTag{
			ID:         localID("tag", entity.ID),
			Provider:   store.ProviderIntercom,
			ProviderID: entity.ID,
			Name:       entity.Name,
			Raw:        entity.Raw,
		})
	}
	return tags
}

func normalizeContacts(entities []intercom.Entity) []store.Contact {
	contacts := make([]store.Contact, 0, len(entities))
	for _, entity := range entities {
		contacts = append(contacts, store.Contact{
			ID:         localID("contact", entity.ID),
			Provider:   store.ProviderIntercom,
			ProviderID: entity.ID,
			Name:       entity.Name,
			Email:      entity.Email,
			Raw:        entity.Raw,
		})
	}
	return contacts
}

func (s IntercomSyncer) sleep(ctx context.Context, delay time.Duration) error {
	if s.Client.Sleep != nil {
		return s.Client.Sleep(ctx, delay)
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

func (s IntercomSyncer) workspace() store.Workspace {
	workspace := s.Workspace
	if workspace.ID == "" {
		workspace.ID = defaultWorkspaceID
	}
	if workspace.Provider == "" {
		workspace.Provider = store.ProviderIntercom
	}
	if workspace.Name == "" {
		workspace.Name = "Intercom"
	}
	if workspace.CreatedAt == "" {
		now := time.Now
		if s.Now != nil {
			now = s.Now
		}
		workspace.CreatedAt = now().UTC().Format(time.RFC3339)
	}
	return workspace
}

func NormalizeConversation(conversation intercom.Conversation) (store.Conversation, error) {
	var raw map[string]any
	if err := json.Unmarshal(conversation.Raw, &raw); err != nil {
		return store.Conversation{}, fmt.Errorf("decode intercom conversation raw: %w", err)
	}
	providerID := firstNonEmpty(conversation.ID, stringValue(raw, "id"))
	if providerID == "" {
		return store.Conversation{}, fmt.Errorf("intercom conversation id is required")
	}
	parts := normalizeParts(providerID, raw)
	participants := normalizeParticipants(raw, parts)
	tags := normalizeTags(raw)
	return store.Conversation{
		ID:           localID("conversation", providerID),
		Provider:     store.ProviderIntercom,
		ProviderID:   providerID,
		Subject:      conversationSubject(raw, parts),
		State:        firstNonEmpty(stringValue(raw, "state"), stringValue(raw, "status"), "unknown"),
		Assignee:     normalizeAssignee(raw),
		Rating:       normalizeRating(raw),
		FinStatus:    normalizeFinStatus(raw),
		Participants: participants,
		Tags:         tags,
		CreatedAt:    timeValue(raw, "created_at"),
		UpdatedAt:    firstNonEmpty(timeValue(raw, "updated_at"), timeValue(raw, "created_at")),
		Parts:        parts,
		Raw:          raw,
	}, nil
}

func normalizeParts(conversationID string, raw map[string]any) []store.Part {
	var parts []store.Part
	if source, ok := mapValue(raw, "source"); ok {
		if body := strings.TrimSpace(stringValue(source, "body")); body != "" {
			partID := firstNonEmpty(stringValue(source, "id"), conversationID+":source")
			parts = append(parts, store.Part{
				ID:         localID("part", partID),
				ProviderID: partID,
				Type:       firstNonEmpty(stringValue(source, "type"), "source"),
				AuthorName: authorName(source),
				Body:       body,
				CreatedAt:  firstNonEmpty(timeValue(source, "created_at"), timeValue(raw, "created_at")),
				UpdatedAt:  firstNonEmpty(timeValue(source, "updated_at"), timeValue(source, "created_at"), timeValue(raw, "created_at")),
				Raw:        source,
			})
		}
	}
	if wrapper, ok := mapValue(raw, "conversation_parts"); ok {
		for _, item := range arrayValue(wrapper, "conversation_parts") {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			partID := stringValue(part, "id")
			if partID == "" {
				partID = conversationID + ":part:" + fmt.Sprint(len(parts)+1)
			}
			parts = append(parts, store.Part{
				ID:         localID("part", partID),
				ProviderID: partID,
				Type:       firstNonEmpty(stringValue(part, "part_type"), stringValue(part, "type"), "part"),
				AuthorName: authorName(part),
				Body:       strings.TrimSpace(stringValue(part, "body")),
				CreatedAt:  timeValue(part, "created_at"),
				UpdatedAt:  firstNonEmpty(timeValue(part, "updated_at"), timeValue(part, "created_at")),
				Raw:        part,
			})
		}
	}
	return parts
}

func normalizeParticipants(raw map[string]any, parts []store.Part) []string {
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}
	if source, ok := mapValue(raw, "source"); ok {
		add(authorName(source))
	}
	if contacts, ok := mapValue(raw, "contacts"); ok {
		for _, item := range arrayValue(contacts, "contacts") {
			contact, ok := item.(map[string]any)
			if !ok {
				continue
			}
			add(firstNonEmpty(stringValue(contact, "name"), stringValue(contact, "email"), stringValue(contact, "id")))
		}
	}
	for _, part := range parts {
		add(part.AuthorName)
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func normalizeTags(raw map[string]any) []string {
	var tags []string
	if wrapper, ok := mapValue(raw, "tags"); ok {
		for _, item := range arrayValue(wrapper, "tags") {
			tag, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if name := strings.TrimSpace(stringValue(tag, "name")); name != "" {
				tags = append(tags, name)
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func normalizeAssignee(raw map[string]any) string {
	for _, field := range []string{"assignee", "admin_assignee", "team_assignee"} {
		if assignee, ok := mapValue(raw, field); ok {
			if name := firstNonEmpty(stringValue(assignee, "name"), stringValue(assignee, "id")); name != "" {
				return name
			}
		}
	}
	return firstNonEmpty(stringValue(raw, "admin_assignee_id"), stringValue(raw, "team_assignee_id"))
}

func normalizeRating(raw map[string]any) string {
	if rating, ok := mapValue(raw, "conversation_rating"); ok {
		return firstNonEmpty(stringValue(rating, "rating"), stringValue(rating, "remark"))
	}
	return ""
}

func normalizeFinStatus(raw map[string]any) string {
	for _, field := range []string{"fin_status", "ai_agent_status"} {
		if value := stringValue(raw, field); value != "" {
			return value
		}
	}
	if value, ok := raw["ai_agent_participated"].(bool); ok && value {
		return "participated"
	}
	return ""
}

func conversationSubject(raw map[string]any, parts []store.Part) string {
	if subject := firstNonEmpty(stringValue(raw, "title"), stringValue(raw, "subject")); subject != "" {
		return subject
	}
	if source, ok := mapValue(raw, "source"); ok {
		if subject := stringValue(source, "subject"); subject != "" {
			return subject
		}
	}
	for _, part := range parts {
		if part.Body != "" {
			return firstWords(part.Body, 12)
		}
	}
	return "Intercom conversation"
}

func authorName(raw map[string]any) string {
	if author, ok := mapValue(raw, "author"); ok {
		return firstNonEmpty(stringValue(author, "name"), stringValue(author, "email"), stringValue(author, "id"), stringValue(author, "type"))
	}
	return ""
}

func mapValue(raw map[string]any, key string) (map[string]any, bool) {
	value, ok := raw[key].(map[string]any)
	return value, ok
}

func arrayValue(raw map[string]any, key string) []any {
	value, _ := raw[key].([]any)
	return value
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

func timeValue(raw map[string]any, key string) string {
	switch value := raw[key].(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
		return value
	case float64:
		return time.Unix(int64(value), 0).UTC().Format(time.RFC3339)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstWords(value string, count int) string {
	fields := strings.Fields(value)
	if len(fields) <= count {
		return strings.Join(fields, " ")
	}
	return strings.Join(fields[:count], " ")
}

func localID(kind, providerID string) string {
	sum := sha256.Sum256([]byte(kind + ":" + providerID))
	return store.ProviderIntercom + "_" + kind + "_" + hex.EncodeToString(sum[:8])
}
