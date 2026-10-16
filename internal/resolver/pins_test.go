package resolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/testutil"
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
			want:  []Pin{{Tool: "node", Version: "20", Source: SourceEnginesNode, Range: ">=20.9.0"}},
		},
		{
			name:  "driftr key beats engines.node",
			files: map[string]string{"sub/package.json": `{"driftr": {"node": "22.14.0"}, "engines": {"node": ">=20"}}`},
			want:  []Pin{{Tool: "node", Version: "22.14.0", Source: SourcePackageJSON}},
		},
		{
			name: "explicit pin in a parent beats engines.node below",
			files: map[string]string{
				".driftr.toml":     "[tools]\nnode = \"20.11.0\"\n",
				"sub/package.json": `{"engines": {"node": ">=18"}}`,
			},
			want: []Pin{{Tool: "node", Version: "20.11.0", Source: SourceProject, Dir: "."}},
		},
		{
			name: "nearest engines.node wins when nothing pins node",
			files: map[string]string{
				"package.json":     `{"engines": {"node": ">=20"}}`,
				"sub/package.json": `{"engines": {"node": ">=18"}}`,
			},
			want: []Pin{{Tool: "node", Version: "18", Source: SourceEnginesNode, Range: ">=18"}},
		},
		{
			name: "unparseable engines.node behind an explicit parent pin is never read",
			files: map[string]string{
				".nvmrc":           "20.11.0\n",
				"sub/package.json": `{"engines": {"node": "18 - 20"}}`,
			},
			want: []Pin{{Tool: "node", Version: "20.11.0", Source: SourceNvmrc, Dir: "."}},
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
				if got.Tool != want.Tool || got.Version != want.Version || got.Source != want.Source || got.Range != want.Range {
					t.Errorf("pin %d = %+v, want tool=%s version=%s source=%s", i, got, want.Tool, want.Version, want.Source)
				}
				wantDir := sub
				if want.Dir == "." {
					wantDir = root
				}
				// macOS temp dirs sit behind the /var -> /private/var symlink.
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

// Not requiring an install is the reason ProjectPins exists next to resolveFromProject.
func TestProjectPins_DoesNotRequireInstall(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
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

// Order: nearer directories first, source order within one, engines.node behind every pin.
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
		{Tool: "pnpm", Version: "9.15.0", Source: SourcePackageManager, Dir: sub},
		{Tool: "node", Version: "20", Source: SourceNvmrc, Dir: root},
		{Tool: "node", Version: ">=24", Source: SourceEnginesNode, Dir: sub},
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

// The resolver never reads a file above every pin, so a broken one there must not fail check.
func TestProjectSources_BrokenFileAboveEveryPin(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	toml := filepath.Join(sub, ".driftr.toml")
	if err := os.WriteFile(toml, []byte("[tools]\nnode = \"22.14.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectSources(sub); err == nil {
		t.Error("ProjectSources() should fail while npm, pnpm, yarn and bun still read the broken file")
	}

	all := "[tools]\nnode = \"22.14.0\"\nnpm = \"10.9.0\"\npnpm = \"9.15.0\"\nyarn = \"1.22.22\"\nbun = \"1.2.3\"\n"
	if err := os.WriteFile(toml, []byte(all), 0o644); err != nil {
		t.Fatal(err)
	}
	pins, err := ProjectSources(sub)
	if err != nil {
		t.Fatalf("ProjectSources() error: %v", err)
	}
	if len(pins) != 5 {
		t.Errorf("got %+v, want the five .driftr.toml pins", pins)
	}
	if _, err := ProjectPins(sub); err != nil {
		t.Errorf("ProjectPins() error: %v", err)
	}
}

// Install, outdated and prune need only the winner; driftr check reports unreadable files.
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

func TestResolvePin(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	for _, v := range []string{"20.11.0", "22.1.0", "22.14.0", "23.5.0", "26.0.0"} {
		setupFakeInstall(t, home, "node", v)
	}

	tests := []struct {
		spec   string
		want   string
		wantOK bool
	}{
		{"22", "22.14.0", true},
		{"22.1.0", "22.1.0", true},
		{"lts/*", "22.14.0", true},
		{"lts/iron", "20.11.0", true},
		{"lts/argon", "", false},
		{"18", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got, ok, err := ResolvePin("node", tt.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ResolvePin(%q) = %q, %v; want %q, %v", tt.spec, got, ok, tt.want, tt.wantOK)
			}
		})
	}

	if _, _, err := ResolvePin("node", "not-a-version"); err == nil {
		t.Error("ResolvePin accepted an unparsable spec")
	}
}

func TestResolveRange(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	for _, v := range []string{"18.20.0", "20.11.0", "22.14.0", "24.0.0"} {
		setupFakeInstall(t, home, "node", v)
	}

	tests := []struct {
		rng    string
		want   string
		wantOK bool
	}{
		{">=20 <23", "22.14.0", true},
		{"^20.9.0", "20.11.0", true},
		{"18 || 20", "20.11.0", true},
		{">=25", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.rng, func(t *testing.T) {
			got, ok, err := ResolveRange("node", tt.rng)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ResolveRange(%q) = %q, %v; want %q, %v", tt.rng, got, ok, tt.want, tt.wantOK)
			}
		})
	}

	if _, _, err := ResolveRange("node", "18 - 20"); err == nil {
		t.Error("ResolveRange accepted an unparsable range")
	}
}
