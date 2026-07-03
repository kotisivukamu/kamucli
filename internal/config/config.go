package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ClientID     string `yaml:"client_id,omitempty"`
	AccessToken  string `yaml:"access_token,omitempty"`
	RefreshToken string `yaml:"refresh_token,omitempty"`
	IDToken      string `yaml:"id_token,omitempty"`
	// AccessKey is the kamuhub access key minted at login (kamuhub ADR 0006): the
	// credential products + git actually accept. The KamuID tokens above only
	// authenticate to kamuhub to mint/re-mint this. AccessKeyExpiresAt is when it
	// lapses (zero = permanent) — kept so `kamu login` can be re-run before then.
	AccessKey          string            `yaml:"access_key,omitempty"`
	AccessKeyExpiresAt time.Time         `yaml:"access_key_expires_at,omitempty"`
	ActiveOrg          string            `yaml:"active_org,omitempty"`
	Endpoints          Endpoints         `yaml:"endpoints,omitempty"`
	Extras             map[string]string `yaml:"extras,omitempty"`
}

const (
	DefaultClientID = "kamu-cli"
	EnvIssuer       = "KAMU_ISSUER"
	EnvClientID     = "KAMU_CLIENT_ID"
	EnvAccessToken  = "KAMU_ACCESS_TOKEN"
	EnvAccessKey    = "KAMU_ACCESS_KEY"
	EnvKamuhub      = "KAMU_KAMUHUB_URL"

	// DefaultKamuhubBase is the front door (BFF) the CLI mints its access key at.
	DefaultKamuhubBase = "https://app.kamuhub.com"
	// KamuhubAudience is the RFC 8707 resource the CLI requests at login so the
	// KamuID token is an audience-bound JWT the BFF verifies locally (ADR 0006).
	// A stable logical identifier, matched by kamuid (validAudiences) and the BFF.
	KamuhubAudience = "https://app.kamuhub.com"
)

// ResolveAccessKey returns the kamuhub access key to authenticate a product/git
// call with, in precedence order: an explicit --key flag, the KAMU_ACCESS_KEY
// env var, then the key stored by `kamu login`. Empty string when none is set.
func ResolveAccessKey(flag string) string {
	if flag != "" {
		return flag
	}
	if v := os.Getenv(EnvAccessKey); v != "" {
		return v
	}
	if c, err := Load(); err == nil {
		return c.AccessKey
	}
	return ""
}

// ResolveKamuhubBase returns the kamuhub front-door (BFF) base URL: env, config,
// or the default. Trailing slash trimmed.
func (c *Config) ResolveKamuhubBase() string {
	base := DefaultKamuhubBase
	if v := os.Getenv(EnvKamuhub); v != "" {
		base = v
	} else if c.Endpoints.Kamuhub != "" {
		base = c.Endpoints.Kamuhub
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base
}

// ResolveIssuer returns the kamuid issuer URL from env, config, or default.
func (c *Config) ResolveIssuer() string {
	if v := os.Getenv(EnvIssuer); v != "" {
		return v
	}
	if c.Endpoints.Kamuid != "" {
		return c.Endpoints.Kamuid
	}
	return "https://accounts.kamuhub.com"
}

// ResolveClientID returns the OAuth client_id from env, config, or default.
func (c *Config) ResolveClientID() string {
	if v := os.Getenv(EnvClientID); v != "" {
		return v
	}
	if c.ClientID != "" {
		return c.ClientID
	}
	return DefaultClientID
}

type Endpoints struct {
	Kamuid     string `yaml:"kamuid,omitempty"`
	Kamuhub    string `yaml:"kamuhub,omitempty"`
	Kamudb     string `yaml:"kamudb,omitempty"`
	Kamubee    string `yaml:"kamubee,omitempty"`
	Kamudns    string `yaml:"kamudns,omitempty"`
	Kamustatus string `yaml:"kamustatus,omitempty"`
}

func DefaultPath() (string, error) {
	if p := os.Getenv("KAMU_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".kamu", "config.yml"), nil
}

func Load() (*Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}

func Save(c *Config) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("temp config: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("chmod config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

type ctxKey struct{}

func NewContext(ctx context.Context, c *Config) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

func FromContext(ctx context.Context) *Config {
	if c, ok := ctx.Value(ctxKey{}).(*Config); ok {
		return c
	}
	return &Config{}
}
