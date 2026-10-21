package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/stackmade/driftr/v2/internal/ioutil"
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

// SaveProjectTool pins one tool in .driftr.toml. An existing file is edited
// in place, so its comments, layout and other keys stay as they were; only a
// file that cannot be edited that way (no [tools] table, an inline table) is
// rewritten whole.
func SaveProjectTool(dir, tool, version string) error {
	path := filepath.Join(dir, ProjectConfigFile)
	data, err := os.ReadFile(path)
	if err == nil {
		if out, ok := patchToolsKey(string(data), tool, version); ok {
			if err := replaceFile(path, []byte(out)); err != nil {
				return fmt.Errorf("failed to write project config: %w", err)
			}
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read project config: %w", err)
	}

	cfg, err := LoadProject(dir)
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &ProjectConfig{}
	}
	cfg.Tools.SetTool(tool, version)
	return SaveProject(dir, cfg)
}

var (
	tomlTableHeader = regexp.MustCompile(`^\s*\[\s*([^\]]*?)\s*\]`)
	tomlStringValue = regexp.MustCompile(`^(\s*=\s*)("[^"\\]*"|'[^']*')`)
)

// patchToolsKey sets tool in the [tools] table of a TOML document by editing
// its line, or adding one after the table's last key. ok is false when the
// result would not read back as that pin, and the caller rewrites the file.
func patchToolsKey(doc, tool, version string) (string, bool) {
	lines := strings.Split(doc, "\n")
	inTools, header, last := false, -1, -1
	done := false
	for i, line := range lines {
		if m := tomlTableHeader.FindStringSubmatch(line); m != nil {
			inTools = m[1] == "tools"
			if inTools {
				header, last = i, i
			}
			continue
		}
		if !inTools {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		last = i
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		for _, key := range []string{tool, `"` + tool + `"`, "'" + tool + "'"} {
			rest, ok := strings.CutPrefix(trimmed, key)
			if !ok {
				continue
			}
			if m := tomlStringValue.FindStringSubmatchIndex(rest); m != nil {
				lines[i] = indent + key + rest[:m[3]] + strconv.Quote(version) + rest[m[5]:]
				done = true
			}
		}
		if done {
			break
		}
	}
	if header < 0 {
		return "", false
	}
	if !done {
		eol := ""
		if strings.HasSuffix(lines[last], "\r") {
			eol = "\r"
		}
		added := tool + " = " + strconv.Quote(version) + eol
		lines = append(lines[:last+1], append([]string{added}, lines[last+1:]...)...)
	}
	out := strings.Join(lines, "\n")

	var check ProjectConfig
	if err := toml.Unmarshal([]byte(out), &check); err != nil || check.Tools.GetTool(tool) != version {
		return "", false
	}
	return out, true
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
