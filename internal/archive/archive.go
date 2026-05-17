package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/klauspost/compress/zstd"
	"github.com/uinaf/fincrawl/internal/store"
)

const SchemaVersion = "fincrawl.archive.v1"

type Record struct {
	SchemaVersion  string         `json:"schema_version"`
	RecordType     string         `json:"record_type"`
	ID             string         `json:"id"`
	Provider       string         `json:"provider"`
	ProviderID     string         `json:"provider_id"`
	ConversationID string         `json:"conversation_id,omitempty"`
	Subject        string         `json:"subject,omitempty"`
	State          string         `json:"state,omitempty"`
	Name           string         `json:"name,omitempty"`
	Email          string         `json:"email,omitempty"`
	TeamIDs        []string       `json:"team_ids,omitempty"`
	Assignee       string         `json:"assignee,omitempty"`
	Rating         string         `json:"rating,omitempty"`
	FinStatus      string         `json:"fin_status,omitempty"`
	Participants   []string       `json:"participants,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	PartType       string         `json:"part_type,omitempty"`
	AuthorName     string         `json:"author_name,omitempty"`
	Body           string         `json:"body,omitempty"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	Raw            map[string]any `json:"raw,omitempty"`
}

func FixtureRecords(fixture store.Fixture) []Record {
	var records []Record
	for _, admin := range fixture.Entities.Admins {
		records = append(records, Record{
			SchemaVersion: SchemaVersion,
			RecordType:    "admin",
			ID:            admin.ID,
			Provider:      admin.Provider,
			ProviderID:    admin.ProviderID,
			Name:          admin.Name,
			Email:         admin.Email,
			TeamIDs:       sortedStrings(admin.TeamIDs),
			Raw:           admin.Raw,
		})
	}
	for _, team := range fixture.Entities.Teams {
		records = append(records, Record{
			SchemaVersion: SchemaVersion,
			RecordType:    "team",
			ID:            team.ID,
			Provider:      team.Provider,
			ProviderID:    team.ProviderID,
			Name:          team.Name,
			Raw:           team.Raw,
		})
	}
	for _, tag := range fixture.Entities.Tags {
		records = append(records, Record{
			SchemaVersion: SchemaVersion,
			RecordType:    "provider_tag",
			ID:            tag.ID,
			Provider:      tag.Provider,
			ProviderID:    tag.ProviderID,
			Name:          tag.Name,
			Raw:           tag.Raw,
		})
	}
	for _, contact := range fixture.Entities.Contacts {
		records = append(records, Record{
			SchemaVersion: SchemaVersion,
			RecordType:    "contact",
			ID:            contact.ID,
			Provider:      contact.Provider,
			ProviderID:    contact.ProviderID,
			Name:          contact.Name,
			Email:         contact.Email,
			Raw:           contact.Raw,
		})
	}
	conversations := append([]store.Conversation(nil), fixture.Conversations...)
	sort.Slice(conversations, func(i, j int) bool {
		if conversations[i].UpdatedAt == conversations[j].UpdatedAt {
			return conversations[i].ProviderID < conversations[j].ProviderID
		}
		return conversations[i].UpdatedAt < conversations[j].UpdatedAt
	})
	for _, conversation := range conversations {
		records = append(records, Record{
			SchemaVersion: SchemaVersion,
			RecordType:    "conversation",
			ID:            conversation.ID,
			Provider:      conversation.Provider,
			ProviderID:    conversation.ProviderID,
			Subject:       conversation.Subject,
			State:         conversation.State,
			Assignee:      conversation.Assignee,
			Rating:        conversation.Rating,
			FinStatus:     conversation.FinStatus,
			Participants:  sortedStrings(conversation.Participants),
			Tags:          sortedStrings(conversation.Tags),
			CreatedAt:     normalizeTime(conversation.CreatedAt),
			UpdatedAt:     normalizeTime(conversation.UpdatedAt),
			Raw:           conversation.Raw,
		})
		parts := append([]store.Part(nil), conversation.Parts...)
		sort.Slice(parts, func(i, j int) bool {
			if parts[i].UpdatedAt == parts[j].UpdatedAt {
				return parts[i].ProviderID < parts[j].ProviderID
			}
			return parts[i].UpdatedAt < parts[j].UpdatedAt
		})
		for _, part := range parts {
			records = append(records, Record{
				SchemaVersion:  SchemaVersion,
				RecordType:     "conversation_part",
				ID:             part.ID,
				Provider:       conversation.Provider,
				ProviderID:     part.ProviderID,
				ConversationID: conversation.ID,
				PartType:       part.Type,
				AuthorName:     part.AuthorName,
				Body:           part.Body,
				CreatedAt:      normalizeTime(part.CreatedAt),
				UpdatedAt:      normalizeTime(part.UpdatedAt),
				Raw:            part.Raw,
			})
		}
	}
	sortRecords(records)
	return records
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		left := recordSortKey(records[i])
		right := recordSortKey(records[j])
		for index := range left {
			if left[index] == right[index] {
				continue
			}
			return left[index] < right[index]
		}
		return false
	})
}

func recordSortKey(record Record) [5]string {
	return [5]string{
		record.RecordType,
		record.ProviderID,
		record.UpdatedAt,
		record.CreatedAt,
		record.ID,
	}
}

func WriteJSONL(w io.Writer, records []Record) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

func JSONLBytes(records []Record) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteJSONL(&buf, records); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func WriteEncryptedJSONL(path, recipientText string, records []Record) error {
	recipient, err := ParseRecipient(recipientText)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer file.Close()
	ageWriter, err := age.Encrypt(file, recipient)
	if err != nil {
		return fmt.Errorf("create age writer: %w", err)
	}
	zstdWriter, err := zstd.NewWriter(ageWriter)
	if err != nil {
		_ = ageWriter.Close()
		return fmt.Errorf("create zstd writer: %w", err)
	}
	if err := WriteJSONL(zstdWriter, records); err != nil {
		_ = zstdWriter.Close()
		_ = ageWriter.Close()
		return err
	}
	if err := zstdWriter.Close(); err != nil {
		_ = ageWriter.Close()
		return fmt.Errorf("close zstd writer: %w", err)
	}
	if err := ageWriter.Close(); err != nil {
		return fmt.Errorf("close age writer: %w", err)
	}
	return nil
}

func ParseRecipient(recipientText string) (age.Recipient, error) {
	recipientText = strings.TrimSpace(recipientText)
	if recipientText == "" {
		return nil, fmt.Errorf("age recipient is required")
	}
	if strings.HasPrefix(recipientText, "age1") {
		recipient, err := age.ParseX25519Recipient(recipientText)
		if err != nil {
			return nil, fmt.Errorf("parse age recipient: %w", err)
		}
		return recipient, nil
	}
	if strings.HasPrefix(recipientText, "ssh-") {
		recipient, err := agessh.ParseRecipient(recipientText)
		if err != nil {
			return nil, fmt.Errorf("parse ssh recipient: %w", err)
		}
		return recipient, nil
	}
	return nil, fmt.Errorf("unsupported age recipient format")
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func normalizeTime(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(time.RFC3339)
}
