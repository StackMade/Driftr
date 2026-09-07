package resolver

import (
	"os"
	"path/filepath"
	"testing"
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
