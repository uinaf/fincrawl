package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Lock struct {
	path string
	file *os.File
}

func Acquire(dbPath string) (*Lock, error) {
	if dbPath == "" {
		return nil, errors.New("lock path requires database path")
	}
	lockPath := dbPath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("store is locked: %s", lockPath)
		}
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		_ = file.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write lock: %w", err)
	}
	return &Lock{path: lockPath, file: file}, nil
}

func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	var closeErr error
	if l.file != nil {
		closeErr = l.file.Close()
	}
	removeErr := os.Remove(l.path)
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	return nil
}
