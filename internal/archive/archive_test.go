package archive

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
	"github.com/uinaf/fincrawl/internal/store"
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
