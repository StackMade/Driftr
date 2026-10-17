package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/platform"
)

// LoadProjects returns the project directories driftr has recorded, in the
// order they were added, each once. A missing registry is not an error: it
// returns nil.
func LoadProjects() ([]string, error) {
	path, err := platform.ProjectsRegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read project registry: %w", err)
	}

	var dirs []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		// Concurrent AddProject calls can each append the same directory.
		if dir := filepath.Clean(line); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// AddProject records dir in the registry. A directory already listed is left
// alone, so recording on every pin and install keeps the file the same size.
// It appends in a single O_APPEND write instead of rewriting the file, so
// concurrent driftr processes cannot drop each other's projects.
func AddProject(dir string) error {
	dirs, err := LoadProjects()
	if err != nil {
		return err
	}
	dir = filepath.Clean(dir)
	if slices.Contains(dirs, dir) {
		return nil
	}
	path, err := platform.ProjectsRegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write project registry: %w", err)
	}
	_, err = f.WriteString(dir + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("failed to write project registry: %w", err)
	}
	return nil
}

// RemoveProjects drops every directory in gone from the registry. It re-reads
// the file first, so a project recorded meanwhile by another driftr process
// survives.
func RemoveProjects(gone []string) error {
	dirs, err := LoadProjects()
	if err != nil {
		return err
	}
	kept := slices.DeleteFunc(dirs, func(d string) bool { return slices.Contains(gone, d) })
	return saveProjects(kept)
}

// saveProjects replaces the registry in one rename, so a crash or a parallel
// reader never sees a half-written list.
func saveProjects(dirs []string) error {
	path, err := platform.ProjectsRegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	var b strings.Builder
	for _, d := range dirs {
		b.WriteString(d)
		b.WriteByte('\n')
	}
	if err := ioutil.WriteFileAtomic(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write project registry: %w", err)
	}
	return nil
}
