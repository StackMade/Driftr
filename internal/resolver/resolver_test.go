package resolver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/platform"
)

// setupFakeInstall creates a fake tool installation with a binary file.
func setupFakeInstall(t *testing.T, home, tool, version string) {
	t.Helper()
	dir, _ := platform.ToolVersionDir(tool, version)
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	// Determine binary name from tool map.
	entry, ok := platform.LookupTool(tool)
	binName := tool
	if ok {
		binName = entry.Binary
	}
	if err := os.WriteFile(filepath.Join(binDir, binName), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}
}

func TestRequireToolInstalled_ExactVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	ver, binPath, err := RequireToolInstalled("node", "22.14.0")
	if err != nil {
		t.Fatalf("RequireToolInstalled() error: %v", err)
	}
	if ver != "22.14.0" {
		t.Errorf("version = %q, want %q", ver, "22.14.0")
	}
	if binPath == "" {
		t.Error("binPath is empty")
	}
}

func TestRequireToolInstalled_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, _, err := RequireToolInstalled("node", "99.0.0")
	if err == nil {
		t.Fatal("expected error for uninstalled version, got nil")
	}
}

func TestRequireToolInstalled_PartialVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")
	setupFakeInstall(t, home, "node", "22.13.0")

	ver, _, err := RequireToolInstalled("node", "22")
	if err != nil {
		t.Fatalf("RequireToolInstalled() error: %v", err)
	}
	if ver != "22.14.0" {
		t.Errorf("version = %q, want %q (latest 22.x)", ver, "22.14.0")
	}
}

func TestRequireToolInstalled_Latest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "20.11.0")
	setupFakeInstall(t, home, "node", "22.14.0")

	ver, _, err := RequireToolInstalled("node", "latest")
	if err != nil {
		t.Fatalf("RequireToolInstalled() error: %v", err)
	}
	if ver != "22.14.0" {
		t.Errorf("version = %q, want %q (latest)", ver, "22.14.0")
	}
}

func TestRequireToolInstalled_PartialNoMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "20.11.0")

	_, _, err := RequireToolInstalled("node", "22")
	if err == nil {
		t.Fatal("expected error for no matching version, got nil")
	}
}

func TestResolveFromProject_DriftrToml(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	// Create project with .driftr.toml.
	projectDir := t.TempDir()
	cfg := &config.ProjectConfig{}
	cfg.Tools.SetTool("node", "22.14.0")
	if err := config.SaveProject(projectDir, cfg); err != nil {
		t.Fatalf("SaveProject() error: %v", err)
	}

	// Chdir to project.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "22.14.0" {
		t.Errorf("version = %q, want %q", res.Version, "22.14.0")
	}
	if res.Source != SourceProject {
		t.Errorf("source = %v, want %v", res.Source, SourceProject)
	}
}

func TestResolveFromProject_PackageJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	// Create project with package.json.
	projectDir := t.TempDir()
	pkgJSON := `{"name": "test", "driftr": {"node": "22.14.0"}}`
	os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(pkgJSON), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "22.14.0" {
		t.Errorf("version = %q, want %q", res.Version, "22.14.0")
	}
	if res.Source != SourcePackageJSON {
		t.Errorf("source = %v, want %v", res.Source, SourcePackageJSON)
	}
}

func TestResolveFromProject_TomlTakesPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "20.0.0")
	setupFakeInstall(t, home, "node", "22.14.0")

	// Create project with both .driftr.toml and package.json.
	projectDir := t.TempDir()
	cfg := &config.ProjectConfig{}
	cfg.Tools.SetTool("node", "20.0.0")
	config.SaveProject(projectDir, cfg)
	pkgJSON := `{"name": "test", "driftr": {"node": "22.14.0"}}`
	os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(pkgJSON), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	// .driftr.toml should win.
	if res.Version != "20.0.0" {
		t.Errorf("version = %q, want %q (.driftr.toml should take priority)", res.Version, "20.0.0")
	}
	if res.Source != SourceProject {
		t.Errorf("source = %v, want %v", res.Source, SourceProject)
	}
}

func TestResolveFromGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := platform.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	setupFakeInstall(t, home, "node", "22.14.0")

	// Set global default.
	globalCfg, _ := config.LoadGlobal()
	globalCfg.Default.SetTool("node", "22.14.0")
	config.SaveGlobal(globalCfg)

	// Chdir to a directory with no project config.
	emptyDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(emptyDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "22.14.0" {
		t.Errorf("version = %q, want %q", res.Version, "22.14.0")
	}
	if res.Source != SourceGlobal {
		t.Errorf("source = %v, want %v", res.Source, SourceGlobal)
	}
}

func TestResolveExplicit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "24.0.0")

	res, err := ResolveTool("node", "24.0.0", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "24.0.0" {
		t.Errorf("version = %q, want %q", res.Version, "24.0.0")
	}
	if res.Source != SourceExplicit {
		t.Errorf("source = %v, want %v", res.Source, SourceExplicit)
	}
}

func TestResolveNoConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	emptyDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(emptyDir)

	_, err := ResolveTool("node", "", false)
	if err == nil {
		t.Fatal("expected error when no config exists, got nil")
	}
}

func TestResolveTool_PnpmIndependent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "pnpm", "9.15.0")

	// Create project with pnpm pinned.
	projectDir := t.TempDir()
	cfg := &config.ProjectConfig{}
	cfg.Tools.SetTool("pnpm", "9.15.0")
	config.SaveProject(projectDir, cfg)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("pnpm", "", false)
	if err != nil {
		t.Fatalf("ResolveTool(pnpm) error: %v", err)
	}
	if res.Tool != "pnpm" {
		t.Errorf("tool = %q, want %q", res.Tool, "pnpm")
	}
	if res.Version != "9.15.0" {
		t.Errorf("version = %q, want %q", res.Version, "9.15.0")
	}
}

func TestResolveBinary_NpmResolveViaNode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := platform.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	setupFakeInstall(t, home, "node", "22.14.0")
	// npm binary is inside the node installation.
	npmBinDir := filepath.Join(home, ".driftr", "tools", "node", "22.14.0", "bin")
	os.WriteFile(filepath.Join(npmBinDir, "npm"), []byte("#!/bin/sh\n"), 0o755)

	globalCfg, _ := config.LoadGlobal()
	globalCfg.Default.SetTool("node", "22.14.0")
	config.SaveGlobal(globalCfg)

	emptyDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(emptyDir)

	binPath, err := ResolveBinary("npm", "")
	if err != nil {
		t.Fatalf("ResolveBinary(npm) error: %v", err)
	}
	if binPath == "" {
		t.Error("binPath is empty")
	}
}

func TestResolveBinaryFull_ExplicitPinsNodeNotTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := platform.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	setupFakeInstall(t, home, "node", "22.14.0")
	setupFakeInstall(t, home, "node", "20.0.0")
	setupFakeInstall(t, home, "pnpm", "9.15.0")

	globalCfg, _ := config.LoadGlobal()
	globalCfg.Default.SetTool("node", "22.14.0")
	globalCfg.Default.SetTool("pnpm", "9.15.0")
	config.SaveGlobal(globalCfg)

	emptyDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(emptyDir)

	// explicit ("20.0.0") comes from `driftr run --node 20.0.0 -- pnpm`.
	// It must pin the Node.js runtime, not pnpm's own version — pnpm still
	// resolves to its configured default (9.15.0).
	rb, err := ResolveBinaryFull("pnpm", "20.0.0")
	if err != nil {
		t.Fatalf("ResolveBinaryFull(pnpm, 20.0.0) error: %v", err)
	}
	if !strings.Contains(rb.ToolPath, filepath.Join("pnpm", "9.15.0")) {
		t.Errorf("ToolPath = %q, want pnpm 9.15.0 (own version unaffected by explicit)", rb.ToolPath)
	}
	if !strings.Contains(rb.NodePath, filepath.Join("node", "20.0.0")) {
		t.Errorf("NodePath = %q, want node 20.0.0 (pinned by explicit)", rb.NodePath)
	}
}

func TestResolveTool_PackageManagerField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "pnpm", "9.15.0")

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"),
		[]byte(`{"name":"app","packageManager":"pnpm@9.15.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("pnpm", "", false)
	if err != nil {
		t.Fatalf("ResolveTool(pnpm) error: %v", err)
	}
	if res.Version != "9.15.0" {
		t.Errorf("version = %q, want %q", res.Version, "9.15.0")
	}
	if res.Source != SourcePackageManager {
		t.Errorf("source = %v, want SourcePackageManager", res.Source)
	}
}

func TestResolveTool_DriftrKeyBeatsPackageManagerField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "pnpm", "9.15.0")
	setupFakeInstall(t, home, "pnpm", "8.0.0")

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"),
		[]byte(`{"name":"app","packageManager":"pnpm@8.0.0","driftr":{"pnpm":"9.15.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("pnpm", "", false)
	if err != nil {
		t.Fatalf("ResolveTool(pnpm) error: %v", err)
	}
	if res.Version != "9.15.0" {
		t.Errorf("version = %q, want %q (driftr key must win over packageManager field)", res.Version, "9.15.0")
	}
	if res.Source != SourcePackageJSON {
		t.Errorf("source = %v, want SourcePackageJSON", res.Source)
	}
}

func TestResolveTool_PackageManagerFieldIgnoredForNode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	globalCfg, _ := config.LoadGlobal()
	globalCfg.Default.SetTool("node", "22.14.0")
	config.SaveGlobal(globalCfg)

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"),
		[]byte(`{"name":"app","packageManager":"pnpm@9.15.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	// A packageManager field naming pnpm must not affect node resolution —
	// it should fall through to the global default.
	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool(node) error: %v", err)
	}
	if res.Source != SourceGlobal {
		t.Errorf("source = %v, want SourceGlobal", res.Source)
	}
}

func TestSourceString(t *testing.T) {
	tests := []struct {
		source Source
		want   string
	}{
		{SourceExplicit, "explicit override"},
		{SourceProject, "project config"},
		{SourcePackageJSON, "package.json (driftr)"},
		{SourceNvmrc, ".nvmrc"},
		{SourceNodeVersion, ".node-version"},
		{SourcePackageManager, "package.json (packageManager)"},
		{SourceEnginesNode, "package.json (engines.node)"},
		{SourceGlobal, "global default"},
		{Source(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.source.String(); got != tt.want {
			t.Errorf("Source(%d).String() = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestNotInstalledError(t *testing.T) {
	e := &NotInstalledError{Tool: "node", Version: "24.1.0", Context: "pinned in /home/user/project"}
	msg := e.Error()
	if !strings.Contains(msg, "node 24.1.0") {
		t.Errorf("error should contain tool and version, got: %s", msg)
	}
	if !strings.Contains(msg, "pinned in /home/user/project") {
		t.Errorf("error should contain context, got: %s", msg)
	}
	if !strings.Contains(msg, "driftr install node@24.1.0") {
		t.Errorf("error should contain install hint, got: %s", msg)
	}

	// Without context.
	e2 := &NotInstalledError{Tool: "pnpm", Version: "9.0.0"}
	msg2 := e2.Error()
	if strings.Contains(msg2, "(") {
		t.Errorf("error without context should not contain parens, got: %s", msg2)
	}
}

func TestNotInstalledError_ErrorsAs(t *testing.T) {
	orig := &NotInstalledError{Tool: "node", Version: "22.0.0"}
	wrapped := fmt.Errorf("resolution failed: %w", orig)

	var target *NotInstalledError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should unwrap NotInstalledError from wrapped error")
	}
	if target.Tool != "node" || target.Version != "22.0.0" {
		t.Errorf("unwrapped error has wrong fields: %+v", target)
	}
}

func TestRequireToolBinaryExists_NotInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := requireToolBinaryExists("node", "99.99.99", "global default")
	if err == nil {
		t.Fatal("expected error for non-existent version")
	}

	var notInstalled *NotInstalledError
	if !errors.As(err, &notInstalled) {
		t.Fatalf("expected NotInstalledError, got %T: %v", err, err)
	}
	if notInstalled.Tool != "node" || notInstalled.Version != "99.99.99" || notInstalled.Context != "global default" {
		t.Errorf("unexpected fields: %+v", notInstalled)
	}
}

func TestResolveFromProject_Nvmrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".nvmrc"), []byte("22.14.0\n"), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "22.14.0" {
		t.Errorf("version = %q, want %q", res.Version, "22.14.0")
	}
	if res.Source != SourceNvmrc {
		t.Errorf("source = %v, want %v", res.Source, SourceNvmrc)
	}
}

func TestResolveFromProject_NodeVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".node-version"), []byte("22.14.0\n"), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "22.14.0" {
		t.Errorf("version = %q, want %q", res.Version, "22.14.0")
	}
	if res.Source != SourceNodeVersion {
		t.Errorf("source = %v, want %v", res.Source, SourceNodeVersion)
	}
}

func TestResolveFromProject_NvmrcPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "20.0.0")
	setupFakeInstall(t, home, "node", "22.14.0")

	// .nvmrc should lose to .driftr.toml
	projectDir := t.TempDir()
	cfg := &config.ProjectConfig{}
	cfg.Tools.SetTool("node", "20.0.0")
	config.SaveProject(projectDir, cfg)
	os.WriteFile(filepath.Join(projectDir, ".nvmrc"), []byte("22.14.0\n"), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "20.0.0" {
		t.Errorf("version = %q, want %q (.driftr.toml should beat .nvmrc)", res.Version, "20.0.0")
	}
	if res.Source != SourceProject {
		t.Errorf("source = %v, want %v", res.Source, SourceProject)
	}
}

func TestResolveFromProject_NvmrcIgnoredForNonNode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .nvmrc but resolve pnpm — should not match.
	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".nvmrc"), []byte("22.14.0\n"), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	// pnpm resolution should fall through to global (and fail with no global set).
	_, err := ResolveTool("pnpm", "", false)
	if err == nil {
		t.Fatal("expected error for pnpm with only .nvmrc, got nil")
	}
}

func TestResolveFromProject_LTSAlias(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		content   string
		installed []string
		want      string
	}{
		{"lts star picks newest even major", ".nvmrc", "lts/*\n", []string{"21.0.0", "20.11.0", "22.14.0"}, "22.14.0"},
		{"bare lts picks newest even major", ".nvmrc", "lts\n", []string{"20.11.0", "22.14.0"}, "22.14.0"},
		{"lts star ignores odd majors", ".nvmrc", "lts/*\n", []string{"20.11.0", "23.5.0"}, "20.11.0"},
		{"codename picks its major", ".nvmrc", "lts/iron\n", []string{"20.11.0", "22.14.0"}, "20.11.0"},
		{"codename picks newest in major", ".node-version", "lts/jod\n", []string{"22.1.0", "22.14.0", "20.11.0"}, "22.14.0"},
		{"codename is case-insensitive", ".node-version", "LTS/Jod\n", []string{"20.11.0", "22.14.0"}, "22.14.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, v := range tt.installed {
				setupFakeInstall(t, home, "node", v)
			}

			projectDir := t.TempDir()
			os.WriteFile(filepath.Join(projectDir, tt.file), []byte(tt.content), 0o644)

			origDir, _ := os.Getwd()
			defer os.Chdir(origDir)
			os.Chdir(projectDir)

			res, err := ResolveTool("node", "", false)
			if err != nil {
				t.Fatalf("ResolveTool() error: %v", err)
			}
			if res.Version != tt.want {
				t.Errorf("version = %q, want %q", res.Version, tt.want)
			}
		})
	}
}

func TestResolveFromProject_LTSAliasNotInstalled(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		installed   []string
		wantVersion string
	}{
		{"no even major installed", "lts/*\n", []string{"23.5.0"}, "lts"},
		{"codename major missing", "lts/iron\n", []string{"22.14.0"}, "lts/iron"},
		{"nothing installed", "lts/jod\n", nil, "lts/jod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, v := range tt.installed {
				setupFakeInstall(t, home, "node", v)
			}

			projectDir := t.TempDir()
			os.WriteFile(filepath.Join(projectDir, ".nvmrc"), []byte(tt.content), 0o644)

			origDir, _ := os.Getwd()
			defer os.Chdir(origDir)
			os.Chdir(projectDir)

			_, err := ResolveTool("node", "", false)
			var notInstalled *NotInstalledError
			if !errors.As(err, &notInstalled) {
				t.Fatalf("expected NotInstalledError, got %T: %v", err, err)
			}
			if notInstalled.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", notInstalled.Version, tt.wantVersion)
			}
			if !strings.Contains(notInstalled.Error(), "driftr install node@"+tt.wantVersion) {
				t.Errorf("error %q does not carry an actionable install command", notInstalled.Error())
			}
			if !strings.Contains(notInstalled.Context, "pinned in") {
				t.Errorf("context = %q, want it to name the pinning directory", notInstalled.Context)
			}
		})
	}
}

func TestResolveFromProject_UnknownLTSCodenameFallsThrough(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	// Unknown codename in .nvmrc: warn and fall through to .node-version.
	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".nvmrc"), []byte("lts/mithril\n"), 0o644)
	os.WriteFile(filepath.Join(projectDir, ".node-version"), []byte("22.14.0\n"), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Version != "22.14.0" || res.Source != SourceNodeVersion {
		t.Errorf("got %q from %v, want 22.14.0 from .node-version", res.Version, res.Source)
	}
}

func enginesProject(t *testing.T, rangeText string) {
	t.Helper()
	projectDir := t.TempDir()
	body := fmt.Sprintf(`{"name":"app","engines":{"node":%q}}`, rangeText)
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
}
func TestResolveFromProject_EnginesNode(t *testing.T) {
	tests := []struct {
		name      string
		rangeText string
		installed []string
		want      string
	}{
		{"greater or equal picks newest", ">=18", []string{"18.20.4", "20.11.1", "22.14.0"}, "22.14.0"},
		{"caret stays inside the major", "^20.9.0", []string{"20.9.0", "20.18.2", "22.14.0"}, "20.18.2"},
		{"or picks the newest alternative", "18 || 20", []string{"18.20.4", "20.11.1", "22.14.0"}, "20.11.1"},
		{"and is bounded on both sides", ">=16 <21", []string{"14.0.0", "20.11.1", "22.14.0"}, "20.11.1"},
		{"wildcard", "18.x", []string{"18.1.0", "18.20.4", "20.0.0"}, "18.20.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, v := range tt.installed {
				setupFakeInstall(t, home, "node", v)
			}
			enginesProject(t, tt.rangeText)

			res, err := ResolveTool("node", "", false)
			if err != nil {
				t.Fatalf("ResolveTool() error: %v", err)
			}
			if res.Version != tt.want {
				t.Errorf("version = %q, want %q", res.Version, tt.want)
			}
			if res.Source != SourceEnginesNode {
				t.Errorf("source = %v, want %v", res.Source, SourceEnginesNode)
			}
		})
	}
}
func TestResolveFromProject_EnginesNodeLosesToDriftrKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "18.20.4")
	setupFakeInstall(t, home, "node", "22.14.0")

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"),
		[]byte(`{"driftr":{"node":"18.20.4"},"engines":{"node":">=20"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Source != SourcePackageJSON || res.Version != "18.20.4" {
		t.Errorf("got %v %s, want SourcePackageJSON 18.20.4", res.Source, res.Version)
	}
}
func TestResolveFromProject_NoEnginesNodeFallsThroughToGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "22.14.0")

	globalCfg, _ := config.LoadGlobal()
	globalCfg.Default.SetTool("node", "22.14.0")
	config.SaveGlobal(globalCfg)

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"),
		[]byte(`{"name":"app","engines":{"npm":">=9"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projectDir)

	res, err := ResolveTool("node", "", false)
	if err != nil {
		t.Fatalf("ResolveTool() error: %v", err)
	}
	if res.Source != SourceGlobal {
		t.Errorf("source = %v, want SourceGlobal", res.Source)
	}
}
func TestResolveFromProject_EnginesNodeIgnoredForOtherTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "pnpm", "9.15.0")

	globalCfg, _ := config.LoadGlobal()
	globalCfg.Default.SetTool("pnpm", "9.15.0")
	config.SaveGlobal(globalCfg)

	enginesProject(t, ">=18")

	res, err := ResolveTool("pnpm", "", false)
	if err != nil {
		t.Fatalf("ResolveTool(pnpm) error: %v", err)
	}
	if res.Source != SourceGlobal {
		t.Errorf("source = %v, want SourceGlobal", res.Source)
	}
}
func TestResolveFromProject_EnginesNodeNotInstalled(t *testing.T) {
	tests := []struct {
		name      string
		rangeText string
		wantHint  string
	}{
		{"lower bound names its major", ">=18", "driftr install node@18"},
		{"caret names its major", "^20.9.0", "driftr install node@20"},
		{"unbounded below falls back to lts", "<21", "driftr install node@lts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			setupFakeInstall(t, home, "node", "16.20.2")
			if tt.rangeText == "<21" {
				// Make sure nothing at all matches.
				os.RemoveAll(filepath.Join(home, ".driftr", "tools", "node"))
			}
			enginesProject(t, tt.rangeText)

			_, err := ResolveTool("node", "", false)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			var notInstalled *NotInstalledError
			if !errors.As(err, &notInstalled) {
				t.Fatalf("error = %T (%v), want *NotInstalledError", err, err)
			}
			if !strings.Contains(err.Error(), tt.wantHint) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantHint)
			}
		})
	}
}
func TestResolveFromProject_EnginesNodeUnsupportedRange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setupFakeInstall(t, home, "node", "20.11.1")
	enginesProject(t, "18 - 20")

	_, err := ResolveTool("node", "", false)
	if err == nil {
		t.Fatal("expected an error for an unsupported range, got nil")
	}
	for _, want := range []string{"package.json", "18 - 20", "hyphen"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}
