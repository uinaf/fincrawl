package lock

import (
	"os"
	"strings"
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

func TestAcquireRejectsEmptyPath(t *testing.T) {
	if _, err := Acquire(""); err == nil {
		t.Fatalf("empty path should error")
	}
}

func TestAcquireCreatesParentDirAndPID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "deep", "archive.db")
	l, err := Acquire(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	body, err := os.ReadFile(dbPath + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.TrimSpace(string(body))
	if pid == "" || pid == "0" {
		t.Fatalf("lock body = %q", body)
	}
}

func TestReleaseNilLockIsNoOp(t *testing.T) {
	var l *Lock
	if err := l.Release(); err != nil {
		t.Fatalf("nil release = %v", err)
	}
}

func TestReleaseIsIdempotentOnVanishedFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	l, err := Acquire(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dbPath + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("release on vanished lock = %v", err)
	}
}

func TestAcquireRejectsUnwritableParent(t *testing.T) {
	parent := t.TempDir()
	dbDir := filepath.Join(parent, "ro")
	if err := os.Mkdir(dbDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dbDir, 0o755) })
	if _, err := Acquire(filepath.Join(dbDir, "archive.db")); err == nil {
		t.Fatalf("acquire on read-only dir should fail")
	}
}
