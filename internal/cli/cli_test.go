package cli

import (
	"bytes"
	"context"
	"testing"

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
