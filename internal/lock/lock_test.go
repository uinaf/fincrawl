package lock

import (
	"path/filepath"
	"testing"
)

func TestAcquireIsExclusive(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	first, err := Acquire(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if _, err := Acquire(dbPath); err == nil {
		t.Fatalf("second lock unexpectedly succeeded")
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}
