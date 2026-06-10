package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AccessToken  string            `yaml:"access_token,omitempty"`
	RefreshToken string            `yaml:"refresh_token,omitempty"`
	IDToken      string            `yaml:"id_token,omitempty"`
	ActiveOrg    string            `yaml:"active_org,omitempty"`
	Endpoints    Endpoints         `yaml:"endpoints,omitempty"`
	Extras       map[string]string `yaml:"extras,omitempty"`
}

type Endpoints struct {
	Kamuid  string `yaml:"kamuid,omitempty"`
	Kamudb  string `yaml:"kamudb,omitempty"`
	Kamubee string `yaml:"kamubee,omitempty"`
	Kamudns string `yaml:"kamudns,omitempty"`
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
