// Package agents installs a skill that teaches AI coding agents to use driftr.
package agents

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/platform"
)

// Guide is the skill body, also printed for agents without a skills directory.
//
//go:embed guide.md
var Guide string

// managedMarker tells a copy driftr wrote from a skill of the same name the
// user wrote: only marked copies are ever replaced or removed.
const managedMarker = "<!-- Managed by driftr: changes are overwritten. Remove with `driftr agents uninstall`. -->"

const frontmatter = `---
name: driftr
description: "Manage Node.js, npm, pnpm, yarn and bun versions with driftr. Use when installing, pinning or switching a JavaScript toolchain version, when a project has .driftr.toml, .nvmrc, .node-version, packageManager or engines.node, or when node, npm, pnpm, yarn or bun is missing, on the wrong version or fails with a driftr error."
---

`

// known lists each agent, the directory in $HOME whose presence means it is
// installed, and the directory it reads personal skills from. Agents that read
// the shared ~/.agents/skills all use it, so one copy serves them.
//
// shortcut: relocated config (CLAUDE_CONFIG_DIR, XDG_CONFIG_HOME) is ignored,
// add it when someone with such a setup reports a missed agent.
var known = []struct{ name, configDir, skillsDir string }{
	{"claude", ".claude", ".claude/skills"},
	{"codex", ".codex", ".agents/skills"},
	{"copilot", ".copilot", ".agents/skills"},
	{"cursor", ".cursor", ".agents/skills"},
	{"devin", ".config/devin", ".agents/skills"},
	{"gemini", ".gemini", ".agents/skills"},
	{"opencode", ".config/opencode", ".agents/skills"},
	{"windsurf", ".codeium/windsurf", ".codeium/windsurf/skills"},
}

// Harness is one supported agent on this machine.
type Harness struct {
	Name     string
	File     string // absolute path of its SKILL.md
	Detected bool
}

// All returns every supported agent, whether or not it is installed.
func All() ([]Harness, error) {
	home, err := platform.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	harnesses := make([]Harness, 0, len(known))
	for _, k := range known {
		_, statErr := os.Stat(filepath.Join(home, filepath.FromSlash(k.configDir)))
		harnesses = append(harnesses, Harness{
			Name:     k.name,
			File:     filepath.Join(home, filepath.FromSlash(k.skillsDir), "driftr", "SKILL.md"),
			Detected: statErr == nil,
		})
	}
	return harnesses, nil
}

// Status describes what is at a skill path.
type Status string

const (
	Missing  Status = "not installed"
	Current  Status = "installed"
	Outdated Status = "outdated"
	Foreign  Status = "not written by driftr"
)

func skill() string {
	return frontmatter + managedMarker + "\n\n" + Guide
}

// StatusOf reports the state of the skill at file.
func StatusOf(file string) (Status, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return Missing, nil
	}
	if err != nil {
		return "", err
	}
	switch {
	case !strings.Contains(string(data), managedMarker):
		return Foreign, nil
	case string(data) == skill():
		return Current, nil
	}
	return Outdated, nil
}

// Install writes the skill to file, refusing to replace one driftr did not write.
func Install(file string) error {
	status, err := StatusOf(file)
	if err != nil {
		return err
	}
	if status == Foreign {
		return fmt.Errorf("%s was not written by driftr and was left in place. Move it away, then run `driftr agents install`", file)
	}
	if status == Current {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return ioutil.WriteFileAtomic(file, []byte(skill()), 0o644)
}

// Uninstall removes the skill at file if driftr wrote it, and reports whether
// it did. The directory goes too, unless the user keeps other files in it.
func Uninstall(file string) (bool, error) {
	status, err := StatusOf(file)
	if err != nil || status == Missing || status == Foreign {
		return false, err
	}
	if err := os.Remove(file); err != nil {
		return false, err
	}
	_ = os.Remove(filepath.Dir(file))
	return true, nil
}

// Refresh brings copies driftr wrote up to this binary's guide. It runs after
// an upgrade, possibly in front of a tool's own output, so it is silent, and it
// never adds a copy the user did not ask for.
func Refresh() {
	harnesses, err := All()
	if err != nil {
		return
	}
	for _, h := range harnesses {
		if status, err := StatusOf(h.File); err == nil && status == Outdated {
			_ = Install(h.File)
		}
	}
}
