package archive

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
	"github.com/uinaf/fincrawl/internal/store"
	"golang.org/x/crypto/ssh"
)

func TestWriteEncryptedJSONLRoundTrip(t *testing.T) {
	fixture, err := store.LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	records := FixtureRecords(fixture)
	plain, err := JSONLBytes(records)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "archive.jsonl.zst.age")
	if err := WriteEncryptedJSONL(out, identity.Recipient().String(), records); err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer encrypted.Close()
	ageReader, err := age.Decrypt(encrypted, identity)
	if err != nil {
		t.Fatal(err)
	}
	zstdReader, err := zstd.NewReader(ageReader)
	if err != nil {
		t.Fatal(err)
	}
	defer zstdReader.Close()
	decrypted, err := io.ReadAll(zstdReader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decrypted JSONL mismatch")
	}
}

func TestReadEncryptedJSONLRoundTrip(t *testing.T) {
	fixture, err := store.LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	records := FixtureRecords(fixture)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "archive.jsonl.zst.age")
	if err := WriteEncryptedJSONL(out, identity.Recipient().String(), records); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEncryptedJSONL(out, identity.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(records) {
		t.Fatalf("records = %d, want %d", len(got), len(records))
	}
	if got[0].SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q", got[0].SchemaVersion)
	}
}

func TestReadEncryptedJSONLRoundTripWithSSHIdentity(t *testing.T) {
	fixture, err := store.LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	records := FixtureRecords(fixture)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	out := filepath.Join(t.TempDir(), "archive.jsonl.zst.age")
	if err := WriteEncryptedJSONL(out, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublicKey))), records); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEncryptedJSONL(out, string(privatePEM))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(records) {
		t.Fatalf("records = %d, want %d", len(got), len(records))
	}
}

func TestRecordsFixtureRoundTrip(t *testing.T) {
	fixture, err := store.LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	records := FixtureRecords(fixture)
	got, err := RecordsFixture(records)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace.ID != fixture.Workspace.ID {
		t.Fatalf("workspace ID = %q, want %q", got.Workspace.ID, fixture.Workspace.ID)
	}
	if len(got.Conversations) != len(fixture.Conversations) {
		t.Fatalf("conversations = %d, want %d", len(got.Conversations), len(fixture.Conversations))
	}
	if len(got.Conversations[0].Parts) == 0 {
		t.Fatalf("missing conversation parts")
	}
}

func TestJSONLIsDeterministic(t *testing.T) {
	fixture, err := store.LoadFixture(filepath.Join("..", "..", "testdata", "synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	records := FixtureRecords(fixture)
	first, err := JSONLBytes(records)
	if err != nil {
		t.Fatal(err)
	}
	second, err := JSONLBytes(records)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("JSONL output is not deterministic")
	}
}

func TestFixtureRecordsUseCanonicalGlobalOrder(t *testing.T) {
	records := FixtureRecords(store.Fixture{Conversations: []store.Conversation{
		{
			ID:         "conv-b",
			Provider:   "intercom",
			ProviderID: "ic_conv_b",
			CreatedAt:  "2026-01-02T00:00:00Z",
			UpdatedAt:  "2026-01-02T00:00:00Z",
			Parts: []store.Part{
				{
					ID:         "part-a",
					ProviderID: "ic_part_a",
					Type:       "comment",
					CreatedAt:  "2026-01-02T00:01:00Z",
					UpdatedAt:  "2026-01-02T00:01:00Z",
				},
			},
		},
		{
			ID:         "conv-a",
			Provider:   "intercom",
			ProviderID: "ic_conv_a",
			CreatedAt:  "2026-01-01T00:00:00Z",
			UpdatedAt:  "2026-01-01T00:00:00Z",
			Parts: []store.Part{
				{
					ID:         "part-b",
					ProviderID: "ic_part_b",
					Type:       "comment",
					CreatedAt:  "2026-01-01T00:01:00Z",
					UpdatedAt:  "2026-01-01T00:01:00Z",
				},
			},
		},
	}})
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.RecordType+"/"+record.ProviderID)
	}
	want := []string{
		"conversation/ic_conv_a",
		"conversation/ic_conv_b",
		"conversation_part/ic_part_a",
		"conversation_part/ic_part_b",
	}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("record order = %#v, want %#v", got, want)
	}
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestInferWorkspacePicksFirstProvider(t *testing.T) {
	records := []Record{{Provider: "  "}, {Provider: "intercom"}, {Provider: "other"}}
	ws := inferWorkspace(records)
	if ws.ID != "intercom" || ws.Provider != "intercom" || ws.Name != "intercom" {
		t.Fatalf("workspace = %#v", ws)
	}
	none := inferWorkspace([]Record{{Provider: " "}, {Provider: ""}})
	if none.Provider != store.ProviderIntercom {
		t.Fatalf("default workspace = %#v", none)
	}
}

func TestNormalizeTimeKeepsBadInput(t *testing.T) {
	if got := normalizeTime("2026-05-19T01:02:03+02:00"); got != "2026-05-18T23:02:03Z" {
		t.Fatalf("offset normalize = %q", got)
	}
	if got := normalizeTime("not a date"); got != "not a date" {
		t.Fatalf("invalid pass-through = %q", got)
	}
}

func TestParseRecipientRejectsBadInput(t *testing.T) {
	if _, err := ParseRecipient("   "); err == nil {
		t.Fatalf("empty recipient should error")
	}
	if _, err := ParseRecipient("nonsense"); err == nil {
		t.Fatalf("unsupported format should error")
	}
	if _, err := ParseRecipient("age1junk"); err == nil {
		t.Fatalf("invalid age recipient should error")
	}
}

func TestParseIdentitiesRejectsBadInput(t *testing.T) {
	if _, err := ParseIdentities("   "); err == nil {
		t.Fatalf("empty identity should error")
	}
	if _, err := ParseIdentities("AGE-SECRET-KEY-1JUNK"); err == nil {
		t.Fatalf("invalid identity should error")
	}
}

func TestWriteEncryptedJSONLRejectsExistingFile(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "archive.jsonl.zst.age")
	if err := os.WriteFile(out, []byte("preexisting"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteEncryptedJSONL(out, identity.Recipient().String(), nil); err == nil {
		t.Fatalf("expected error for existing file")
	}
}

func TestWriteEncryptedJSONLRejectsBadRecipient(t *testing.T) {
	out := filepath.Join(t.TempDir(), "archive.jsonl.zst.age")
	if err := WriteEncryptedJSONL(out, "not-a-recipient", nil); err == nil {
		t.Fatalf("expected error for bad recipient")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("file should not be created on bad recipient")
	}
}

func TestRecordsFixtureRejectsUnsupportedSchemaVersion(t *testing.T) {
	if _, err := RecordsFixture([]Record{{SchemaVersion: "future", RecordType: "workspace"}}); err == nil {
		t.Fatalf("expected unsupported schema version error")
	}
}

func TestReadEncryptedJSONLRejectsWrongIdentity(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "x.jsonl.zst.age")
	records := []Record{{SchemaVersion: SchemaVersion, RecordType: "workspace", ID: "ws", Provider: "intercom", Name: "ws"}}
	if err := WriteEncryptedJSONL(out, identity.Recipient().String(), records); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEncryptedJSONL(out, wrong.String()); err == nil {
		t.Fatalf("expected decryption failure with wrong identity")
	}
}

func TestReadEncryptedJSONLRejectsMissingFile(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEncryptedJSONL(filepath.Join(t.TempDir(), "nope.jsonl.zst.age"), identity.String()); err == nil {
		t.Fatalf("expected missing file error")
	}
}

func TestReadEncryptedJSONLRejectsBadIdentityString(t *testing.T) {
	if _, err := ReadEncryptedJSONL(filepath.Join(t.TempDir(), "x.jsonl.zst.age"), "  "); err == nil {
		t.Fatalf("expected empty identity error")
	}
}

func TestReadJSONLDecodesAllRecords(t *testing.T) {
	records := []Record{
		{SchemaVersion: SchemaVersion, RecordType: "workspace", ID: "ws", Provider: "intercom", Name: "ws"},
		{SchemaVersion: SchemaVersion, RecordType: "conversation", ID: "c1", Provider: "intercom", ProviderID: "ic_c1"},
	}
	body, err := JSONLBytes(records)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ReadJSONL(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(records) {
		t.Fatalf("parsed %d, want %d", len(parsed), len(records))
	}
}

func TestReadJSONLRejectsBadJSON(t *testing.T) {
	if _, err := ReadJSONL(bytes.NewReader([]byte(`{"bad json`))); err == nil {
		t.Fatalf("expected json error")
	}
}

func TestReadJSONLRejectsUnsupportedSchema(t *testing.T) {
	body := []byte(`{"schema_version":"future","record_type":"workspace"}` + "\n")
	if _, err := ReadJSONL(bytes.NewReader(body)); err == nil {
		t.Fatalf("expected unsupported schema error")
	}
}

func TestJSONLBytesProducesNewlineDelimited(t *testing.T) {
	body, err := JSONLBytes([]Record{
		{SchemaVersion: SchemaVersion, RecordType: "workspace", ID: "a"},
		{SchemaVersion: SchemaVersion, RecordType: "workspace", ID: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Fatalf("expected trailing newline; got %q", body)
	}
	if got := strings.Count(string(body), "\n"); got != 2 {
		t.Fatalf("newlines = %d, want 2", got)
	}
}
