package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/stackmade/driftr/internal/ioutil"
)

const ProjectConfigFile = ".driftr.toml"

// ProjectConfig represents .driftr.toml in a project root.
type ProjectConfig struct {
	Tools ToolsConfig `toml:"tools"`
}

// ToolsConfig holds pinned tool versions for a project.
// Serialized as a flat TOML map: [tools] node = "22.14.0", pnpm = "9.15.0"
type ToolsConfig map[string]string

// GetTool returns the pinned version for a tool.
func (tc ToolsConfig) GetTool(tool string) string {
	if tc == nil {
		return ""
	}
	return tc[tool]
}

// SetTool sets the pinned version for a tool.
func (tc *ToolsConfig) SetTool(tool, version string) {
	if *tc == nil {
		*tc = make(ToolsConfig)
	}
	(*tc)[tool] = version
}

// LoadProject reads the project config from the given directory.
func LoadProject(dir string) (*ProjectConfig, error) {
	path := filepath.Join(dir, ProjectConfigFile)

	cfg := &ProjectConfig{}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read project config: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse project config %s: %w", path, err)
	}

	return cfg, nil
}

// SaveProject writes a project config file in the given directory.
func SaveProject(dir string, cfg *ProjectConfig) error {
	path := filepath.Join(dir, ProjectConfigFile)

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("failed to write project config: %w", err)
	}
	if err := replaceFile(path, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write project config: %w", err)
	}
	return nil
}

// replaceFile writes data to path in one rename, so a shim resolving a
// version meanwhile never reads an empty or half-written config. It keeps the
// mode of the file it replaces, and writes through a symlink, not over it.
func replaceFile(path string, data []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return ioutil.WriteFileAtomic(path, data, mode)
}
