package resolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stackmade/driftr/internal/config"
)

func TestProjectPins(t *testing.T) {
	tests := []struct {
		name string
		// files to write, relative to the temp root; the walk starts at "sub".
		files map[string]string
		want  []Pin
	}{
		{
			name:  "node from .driftr.toml",
			files: map[string]string{"sub/.driftr.toml": "[tools]\nnode = \"22.14.0\"\n"},
			want:  []Pin{{Tool: "node", Version: "22.14.0", Source: SourceProject}},
		},
		{
			name:  "pnpm from packageManager",
			files: map[string]string{"sub/package.json": `{"packageManager": "pnpm@9.15.0"}`},
			want:  []Pin{{Tool: "pnpm", Version: "9.15.0", Source: SourcePackageManager}},
		},
		{
			name:  "bun from packageManager",
			files: map[string]string{"sub/package.json": `{"packageManager": "bun@1.2.3"}`},
			want:  []Pin{{Tool: "bun", Version: "1.2.3", Source: SourcePackageManager}},
		},
		{
			name:  "driftr key beats packageManager",
			files: map[string]string{"sub/package.json": `{"driftr": {"pnpm": "8.0.0"}, "packageManager": "pnpm@9.15.0"}`},
			want:  []Pin{{Tool: "pnpm", Version: "8.0.0", Source: SourcePackageJSON}},
		},
		{
			name:  "node from .nvmrc",
			files: map[string]string{"sub/.nvmrc": "20.11.0\n"},
			want:  []Pin{{Tool: "node", Version: "20.11.0", Source: SourceNvmrc}},
		},
		{
			name:  "LTS alias from .nvmrc is passed through verbatim",
			files: map[string]string{"sub/.nvmrc": "lts/iron\n"},
			want:  []Pin{{Tool: "node", Version: "lts/iron", Source: SourceNvmrc}},
		},
		{
			name:  "engines.node yields its lower-bound major",
			files: map[string]string{"sub/package.json": `{"engines": {"node": ">=20.9.0"}}`},
			want:  []Pin{{Tool: "node", Version: "20", Source: SourceEnginesNode}},
		},
		{
			name:  "driftr key beats engines.node",
			files: map[string]string{"sub/package.json": `{"driftr": {"node": "22.14.0"}, "engines": {"node": ">=20"}}`},
			want:  []Pin{{Tool: "node", Version: "22.14.0", Source: SourcePackageJSON}},
		},
		{
			name:  "no pins",
			files: map[string]string{"sub/README.md": "nothing here"},
			want:  nil,
		},
		{
			name: "pin found in a parent directory",
			files: map[string]string{
				".driftr.toml":     "[tools]\nnode = \"22.14.0\"\n",
				"sub/package.json": `{"packageManager": "yarn@1.22.22"}`,
			},
			want: []Pin{
				{Tool: "node", Version: "22.14.0", Source: SourceProject, Dir: "."},
				{Tool: "yarn", Version: "1.22.22", Source: SourcePackageManager, Dir: "sub"},
			},
		},
		{
			name: "node and pnpm from different sources at the same level",
			files: map[string]string{
				"sub/.nvmrc":       "20.11.0\n",
				"sub/package.json": `{"packageManager": "pnpm@9.15.0"}`,
			},
			want: []Pin{
				{Tool: "node", Version: "20.11.0", Source: SourceNvmrc},
				{Tool: "pnpm", Version: "9.15.0", Source: SourcePackageManager},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				path := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}
			sub := filepath.Join(root, "sub")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			pins, err := ProjectPins(sub)
			if err != nil {
				t.Fatalf("ProjectPins() error: %v", err)
			}
			if len(pins) != len(tt.want) {
				t.Fatalf("got %d pins %+v, want %d", len(pins), pins, len(tt.want))
			}
			for i, want := range tt.want {
				got := pins[i]
				if got.Tool != want.Tool || got.Version != want.Version || got.Source != want.Source {
					t.Errorf("pin %d = %+v, want tool=%s version=%s source=%s", i, got, want.Tool, want.Version, want.Source)
				}
				wantDir := sub
				if want.Dir == "." {
					wantDir = root
				}
				// The temp dir may be a symlink (/var vs /private/var on macOS);
				// compare the resolved paths.
				if !sameDir(t, got.Dir, wantDir) {
					t.Errorf("pin %d dir = %q, want %q", i, got.Dir, wantDir)
				}
			}
		})
	}
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return a == b
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return a == b
	}
	return ra == rb
}

// ProjectPins must not require the version to be installed — that is the whole
// reason it exists next to resolveFromProject.
func TestProjectPins_DoesNotRequireInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftr.toml"), []byte("[tools]\nnode = \"99.0.0\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pins, err := ProjectPins(dir)
	if err != nil {
		t.Fatalf("ProjectPins() error: %v", err)
	}
	if len(pins) != 1 || pins[0].Version != "99.0.0" {
		t.Fatalf("got %+v, want a single node@99.0.0 pin", pins)
	}
}

// ProjectSources keeps every source, not just the winner, in resolution order:
// nearer directories first, then source order within a directory.
func TestProjectSources(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	files := map[string]string{
		".nvmrc":           "20\n",
		"sub/.driftr.toml": "[tools]\nnode = \"22.14.0\"\n",
		"sub/.nvmrc":       "22\n",
		"sub/package.json": `{"engines": {"node": ">=24"}, "packageManager": "pnpm@9.15.0"}`,
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	pins, err := ProjectSources(sub)
	if err != nil {
		t.Fatalf("ProjectSources() error: %v", err)
	}
	want := []Pin{
		{Tool: "node", Version: "22.14.0", Source: SourceProject, Dir: sub},
		{Tool: "node", Version: "22", Source: SourceNvmrc, Dir: sub},
		{Tool: "node", Version: ">=24", Source: SourceEnginesNode, Dir: sub},
		{Tool: "pnpm", Version: "9.15.0", Source: SourcePackageManager, Dir: sub},
		{Tool: "node", Version: "20", Source: SourceNvmrc, Dir: root},
	}
	if len(pins) != len(want) {
		t.Fatalf("got %d pins %+v, want %d", len(pins), pins, len(want))
	}
	for i, w := range want {
		g := pins[i]
		if g.Tool != w.Tool || g.Version != w.Version || g.Source != w.Source || !sameDir(t, g.Dir, w.Dir) {
			t.Errorf("pin %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestProjectSources_BrokenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftr.toml"), []byte("[tools\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := ProjectSources(dir); err == nil {
		t.Fatal("ProjectSources() should report an unparseable .driftr.toml")
	}
}

// Only the winner matters to install, outdated and prune, so a file behind it
// that cannot be read must not fail them. driftr check reads every source and
// reports it.
func TestPins_UnreadableFileBehindWinner(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of their mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftr.toml"), []byte("[tools]\nnode = \"22.14.0\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	nvmrc := filepath.Join(dir, ".nvmrc")
	if err := os.WriteFile(nvmrc, []byte("20\n"), 0o000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(nvmrc, 0o644) })

	ver, src, ok, err := pinAt("node", dir, mustLoadProject(t, dir), nil)
	if err != nil || !ok || ver != "22.14.0" || src != SourceProject {
		t.Errorf("pinAt() = %q, %v, %v, %v; want the .driftr.toml pin", ver, src, ok, err)
	}

	pins, err := ProjectPins(dir)
	if err != nil {
		t.Fatalf("ProjectPins() error: %v", err)
	}
	if len(pins) != 1 || pins[0].Version != "22.14.0" || pins[0].Source != SourceProject {
		t.Errorf("ProjectPins() = %+v, want the .driftr.toml pin", pins)
	}

	if _, err := ProjectSources(dir); err == nil {
		t.Error("ProjectSources() should report the unreadable .nvmrc")
	}
}

func mustLoadProject(t *testing.T, dir string) *config.ProjectConfig {
	t.Helper()
	cfg, err := config.LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	return cfg
}
