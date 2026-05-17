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
	if fixture.Workspace.ID != "" || fixture.Workspace.Provider != "" || fixture.Workspace.Name != "" {
		records = append(records, Record{
			SchemaVersion: SchemaVersion,
			RecordType:    "workspace",
			ID:            fixture.Workspace.ID,
			Provider:      fixture.Workspace.Provider,
			ProviderID:    fixture.Workspace.ID,
			Name:          fixture.Workspace.Name,
			CreatedAt:     normalizeTime(fixture.Workspace.CreatedAt),
		})
	}
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

func ReadEncryptedJSONL(path, identityText string) ([]Record, error) {
	identities, err := ParseIdentities(identityText)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()
	ageReader, err := age.Decrypt(file, identities...)
	if err != nil {
		return nil, fmt.Errorf("decrypt archive: %w", err)
	}
	zstdReader, err := zstd.NewReader(ageReader)
	if err != nil {
		return nil, fmt.Errorf("create zstd reader: %w", err)
	}
	defer zstdReader.Close()
	records, err := ReadJSONL(zstdReader)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func ReadJSONL(r io.Reader) ([]Record, error) {
	decoder := json.NewDecoder(r)
	var records []Record
	lineNo := 0
	for {
		lineNo++
		var record Record
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode JSONL line %d: %w", lineNo, err)
		}
		if record.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("decode JSONL line %d: unsupported schema version %q", lineNo, record.SchemaVersion)
		}
		records = append(records, record)
	}
	return records, nil
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

func ParseIdentities(identityText string) ([]age.Identity, error) {
	identityText = strings.TrimSpace(identityText)
	if identityText == "" {
		return nil, fmt.Errorf("age identity is required")
	}
	if strings.HasPrefix(identityText, "-----BEGIN") {
		identity, err := agessh.ParseIdentity([]byte(identityText))
		if err != nil {
			return nil, fmt.Errorf("parse ssh identity: %w", err)
		}
		return []age.Identity{identity}, nil
	}
	identities, err := age.ParseIdentities(strings.NewReader(identityText))
	if err != nil {
		return nil, fmt.Errorf("parse age identity: %w", err)
	}
	return identities, nil
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

func RecordsFixture(records []Record) (store.Fixture, error) {
	var fixture store.Fixture
	conversations := map[string]int{}
	for _, record := range records {
		if record.SchemaVersion != SchemaVersion {
			return store.Fixture{}, fmt.Errorf("unsupported schema version %q", record.SchemaVersion)
		}
		switch record.RecordType {
		case "workspace":
			fixture.Workspace = store.Workspace{
				ID:        record.ID,
				Provider:  record.Provider,
				Name:      record.Name,
				CreatedAt: record.CreatedAt,
			}
		case "admin":
			fixture.Entities.Admins = append(fixture.Entities.Admins, store.Admin{
				ID:         record.ID,
				Provider:   record.Provider,
				ProviderID: record.ProviderID,
				Name:       record.Name,
				Email:      record.Email,
				TeamIDs:    sortedStrings(record.TeamIDs),
				Raw:        record.Raw,
			})
		case "team":
			fixture.Entities.Teams = append(fixture.Entities.Teams, store.Team{
				ID:         record.ID,
				Provider:   record.Provider,
				ProviderID: record.ProviderID,
				Name:       record.Name,
				Raw:        record.Raw,
			})
		case "provider_tag":
			fixture.Entities.Tags = append(fixture.Entities.Tags, store.ProviderTag{
				ID:         record.ID,
				Provider:   record.Provider,
				ProviderID: record.ProviderID,
				Name:       record.Name,
				Raw:        record.Raw,
			})
		case "contact":
			fixture.Entities.Contacts = append(fixture.Entities.Contacts, store.Contact{
				ID:         record.ID,
				Provider:   record.Provider,
				ProviderID: record.ProviderID,
				Name:       record.Name,
				Email:      record.Email,
				Raw:        record.Raw,
			})
		case "conversation":
			conversation := store.Conversation{
				ID:           record.ID,
				Provider:     record.Provider,
				ProviderID:   record.ProviderID,
				Subject:      record.Subject,
				State:        record.State,
				Assignee:     record.Assignee,
				Rating:       record.Rating,
				FinStatus:    record.FinStatus,
				Participants: sortedStrings(record.Participants),
				Tags:         sortedStrings(record.Tags),
				CreatedAt:    record.CreatedAt,
				UpdatedAt:    record.UpdatedAt,
				Raw:          record.Raw,
			}
			fixture.Conversations = append(fixture.Conversations, conversation)
			conversations[conversation.ID] = len(fixture.Conversations) - 1
		case "conversation_part":
			if strings.TrimSpace(record.ConversationID) == "" {
				return store.Fixture{}, fmt.Errorf("conversation_part %q is missing conversation_id", record.ID)
			}
			index, ok := conversations[record.ConversationID]
			if !ok {
				fixture.Conversations = append(fixture.Conversations, store.Conversation{
					ID:         record.ConversationID,
					Provider:   record.Provider,
					ProviderID: record.ConversationID,
				})
				index = len(fixture.Conversations) - 1
				conversations[record.ConversationID] = index
			}
			fixture.Conversations[index].Parts = append(fixture.Conversations[index].Parts, store.Part{
				ID:         record.ID,
				ProviderID: record.ProviderID,
				Type:       record.PartType,
				AuthorName: record.AuthorName,
				Body:       record.Body,
				CreatedAt:  record.CreatedAt,
				UpdatedAt:  record.UpdatedAt,
				Raw:        record.Raw,
			})
		default:
			return store.Fixture{}, fmt.Errorf("unsupported archive record type %q", record.RecordType)
		}
	}
	if fixture.Workspace.ID == "" {
		fixture.Workspace = inferWorkspace(records)
	}
	return fixture, nil
}

func inferWorkspace(records []Record) store.Workspace {
	provider := store.ProviderIntercom
	for _, record := range records {
		if strings.TrimSpace(record.Provider) != "" {
			provider = record.Provider
			break
		}
	}
	return store.Workspace{
		ID:       provider,
		Provider: provider,
		Name:     provider,
	}
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
