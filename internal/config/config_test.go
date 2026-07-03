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

func TestEnvAccessorsTrimAndReadFromProcess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		key    string
		value  string
		want   string
		getter func() string
	}{
		{"AgeRecipient trimmed", EnvAgeRecipient, "  age1xyz  ", "age1xyz", AgeRecipient},
		{"AgeIdentity trimmed", EnvAgeIdentity, "\tAGE-SECRET-KEY-1\n", "AGE-SECRET-KEY-1", AgeIdentity},
		{"IntercomToken trimmed", EnvIntercomCred, " dG9rZW4= ", "dG9rZW4=", IntercomToken},
		{"IntercomBaseURL trimmed", EnvIntercomBase, "  https://api.eu.intercom.io  ", "https://api.eu.intercom.io", IntercomBaseURL},
		{"IntercomVersion trimmed", EnvIntercomVer, "  2.13  ", "2.13", IntercomVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if got := tc.getter(); got != tc.want {
				t.Fatalf("%s = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestEnsureDirsCreatesRuntimePaths(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv(EnvHome, "runtime")
	t.Setenv(App.ConfigEnv, "")
	rt, err := LoadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if rt.Config.DBPath != filepath.Join(root, "runtime", "fincrawl.db") {
		t.Fatalf("db path = %q, want runtime home under %q", rt.Config.DBPath, root)
	}
	if err := EnsureDirs(rt); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{rt.Config.CacheDir, rt.Config.LogDir, rt.Config.ShareDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %q (err=%v info=%v)", dir, err, info)
		}
	}
}

func TestUnquoteStripsMatchedDelimiters(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{`"value"`, "value"},
		{`'value'`, "value"},
		{`value`, "value"},
		{`""`, ""},
		{`"`, `"`},
		{`'mixed"`, `'mixed"`},
	} {
		if got := unquote(tc.in); got != tc.want {
			t.Fatalf("unquote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLoadDotEnvSetsAndQuoting(t *testing.T) {
	root := t.TempDir()
	body := "# comment\n\n" +
		`PLAIN=hello` + "\n" +
		`QUOTED="quoted value"` + "\n" +
		`SQUOTED='sval'` + "\n" +
		`SKIP_EXISTING=ignored` + "\n"
	path := filepath.Join(root, "env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAIN", "")
	t.Setenv("QUOTED", "")
	t.Setenv("SQUOTED", "")
	t.Setenv("SKIP_EXISTING", "preset")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("PLAIN"); v != "hello" {
		t.Fatalf("PLAIN = %q", v)
	}
	if v := os.Getenv("QUOTED"); v != "quoted value" {
		t.Fatalf("QUOTED = %q", v)
	}
	if v := os.Getenv("SQUOTED"); v != "sval" {
		t.Fatalf("SQUOTED = %q", v)
	}
	if v := os.Getenv("SKIP_EXISTING"); v != "preset" {
		t.Fatalf("SKIP_EXISTING = %q, want preset", v)
	}
}

func TestLoadDotEnvMissingFileIsNoOp(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}
