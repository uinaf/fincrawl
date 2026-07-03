package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ckconfig "github.com/openclaw/crawlkit/config"
)

const (
	AppID           = "fincrawl"
	DisplayName     = "fincrawl"
	EnvHome         = "FINCRAWL_HOME"
	EnvAgeRecipient = "FINCRAWL_AGE_RECIPIENT"
	EnvAgeIdentity  = "FINCRAWL_AGE_IDENTITY"
	EnvIntercomCred = "FINCRAWL_INTERCOM_" + "TOKEN"
	EnvIntercomBase = "FINCRAWL_INTERCOM_BASE_URL"
	EnvIntercomVer  = "FINCRAWL_INTERCOM_VERSION"
)

var App = ckconfig.App{
	Name:         AppID,
	ConfigEnv:    "FINCRAWL_CONFIG",
	PlatformDirs: true,
}

type Runtime struct {
	Paths            ckconfig.Paths
	Config           ckconfig.RuntimeConfig
	AgeRecipientSet  bool
	AgeIdentitySet   bool
	IntercomTokenSet bool
}

func LoadRuntime() (Runtime, error) {
	if err := LoadDotEnv(".env.local"); err != nil {
		return Runtime{}, err
	}
	app := App
	if home := strings.TrimSpace(os.Getenv(EnvHome)); home != "" {
		absHome, err := filepath.Abs(home)
		if err != nil {
			return Runtime{}, err
		}
		app.BaseDir = absHome
		app.PlatformDirs = false
	}
	paths, err := app.DefaultPaths()
	if err != nil {
		return Runtime{}, err
	}
	configPath, err := app.ResolveConfigPath("")
	if err != nil {
		return Runtime{}, err
	}
	paths.ConfigPath = configPath
	defaults, err := app.DefaultRuntimeConfig()
	if err != nil {
		return Runtime{}, err
	}
	cfg := defaults
	if _, err := os.Stat(configPath); err == nil {
		if err := ckconfig.LoadTOML(configPath, &cfg); err != nil {
			return Runtime{}, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return Runtime{}, err
	}
	ckconfig.ApplyRuntimeDefaults(&cfg, defaults)
	return Runtime{
		Paths:            paths,
		Config:           cfg,
		AgeRecipientSet:  strings.TrimSpace(os.Getenv(EnvAgeRecipient)) != "",
		AgeIdentitySet:   strings.TrimSpace(os.Getenv(EnvAgeIdentity)) != "",
		IntercomTokenSet: strings.TrimSpace(os.Getenv(EnvIntercomCred)) != "",
	}, nil
}

func EnsureDirs(rt Runtime) error {
	return ckconfig.EnsureRuntimeDirs(rt.Config)
}

func AgeRecipient() string {
	return strings.TrimSpace(os.Getenv(EnvAgeRecipient))
}

func AgeIdentity() string {
	return strings.TrimSpace(os.Getenv(EnvAgeIdentity))
}

func IntercomToken() string {
	return strings.TrimSpace(os.Getenv(EnvIntercomCred))
}

func IntercomBaseURL() string {
	return strings.TrimSpace(os.Getenv(EnvIntercomBase))
}

func IntercomVersion() string {
	return strings.TrimSpace(os.Getenv(EnvIntercomVer))
}

func LoadDotEnv(path string) error {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("parse %s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		os.Setenv(key, unquote(strings.TrimSpace(value)))
	}
	return scanner.Err()
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
