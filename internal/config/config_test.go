package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeUsesConfiguredRuntimeFile(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	dbPath := filepath.Join(root, "custom.db")
	cacheDir := filepath.Join(root, "cache")
	logDir := filepath.Join(root, "logs")
	shareDir := filepath.Join(root, "share")
	body := "db_path = " + quoteTOML(dbPath) + "\n" +
		"cache_dir = " + quoteTOML(cacheDir) + "\n" +
		"log_dir = " + quoteTOML(logDir) + "\n" +
		"share_dir = " + quoteTOML(shareDir) + "\n"
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(App.ConfigEnv, configPath)
	t.Setenv(EnvHome, "")

	rt, err := LoadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if rt.Paths.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", rt.Paths.ConfigPath, configPath)
	}
	if rt.Config.DBPath != dbPath {
		t.Fatalf("db path = %q, want %q", rt.Config.DBPath, dbPath)
	}
	if rt.Config.CacheDir != cacheDir || rt.Config.LogDir != logDir || rt.Config.ShareDir != shareDir {
		t.Fatalf("runtime dirs = %#v", rt.Config)
	}
}

func TestLoadRuntimeReturnsDotEnvParseErrors(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile(".env.local", []byte("not valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntime(); err == nil {
		t.Fatalf("expected .env.local parse error")
	}
}

func quoteTOML(value string) string {
	return `"` + value + `"`
}
