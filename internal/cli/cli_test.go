package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/openclaw/crawlkit/output"
)

func TestSyncRejectsAmbiguousModes(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--fixture", "testdata/synthetic",
		"--conversation", "conversation_synthetic",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsMissingMode(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"sync"}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSyncRejectsNegativeUpdatedSinceLimit(t *testing.T) {
	t.Setenv("FINCRAWL_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{
		"sync",
		"--updated-since", "2h",
		"--limit=-1",
	}, &stdout, &stderr)
	if !output.IsUsage(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestVersionPrintsJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), []string{"version", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"version": "dev"`)) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestParseSinceAcceptsDayDurations(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	got, err := parseSince("2d", now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("since = %s, want %s", got, want)
	}
}
