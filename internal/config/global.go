package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/stackmade/driftr/internal/platform"
)

// GlobalConfig represents ~/.driftr/config/config.toml
type GlobalConfig struct {
	Default     DefaultConfig `toml:"default"`
	AutoInstall bool          `toml:"auto_install,omitempty"` // install missing versions without prompting
}

// DefaultConfig holds the default tool versions.
type DefaultConfig struct {
	Node  string            `toml:"node"`            // backwards compat: [default] node = "..."
	Tools map[string]string `toml:"tools,omitempty"` // [default.tools] node = "..."
}

// GetTool returns the default version for a tool, checking both the map and legacy field.
func (d *DefaultConfig) GetTool(tool string) string {
	if d.Tools != nil {
		if v, ok := d.Tools[tool]; ok {
			return v
		}
	}
	if tool == "node" {
		return d.Node
	}
	return ""
}

// SetTool sets the default version for a tool.
func (d *DefaultConfig) SetTool(tool, version string) {
	if d.Tools == nil {
		d.Tools = make(map[string]string)
	}
	d.Tools[tool] = version
	// Keep legacy field in sync for backwards compat.
	if tool == "node" {
		d.Node = version
	}
}

// LoadGlobal reads the global configuration file.
func LoadGlobal() (*GlobalConfig, error) {
	path, err := platform.GlobalConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &GlobalConfig{}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	return cfg, nil
}

// SaveGlobal writes the global configuration file.
func SaveGlobal(cfg *GlobalConfig) error {
	path, err := platform.GlobalConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}
	if err := replaceFile(path, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}
	return nil
}
