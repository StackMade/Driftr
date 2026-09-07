package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/config"
)

func TestMigratePin_TOMLToPackageJSON(t *testing.T) {
	dir := t.TempDir()
	toml := filepath.Join(dir, config.ProjectConfigFile)
	if err := os.WriteFile(toml, []byte("[tools]\nnode = \"20.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := migratePin(dir, "node", "22.14.0", formatTOML); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	pkg, err := config.LoadPackageJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pkg == nil || pkg.Driftr.GetTool("node") != "22.14.0" {
		t.Fatalf("package.json does not carry the pin after migration: %+v", pkg)
	}
	// The TOML file must be gone, otherwise it would keep winning over package.json.
	if _, err := os.Stat(toml); !os.IsNotExist(err) {
		t.Errorf("%s still exists after migration", config.ProjectConfigFile)
	}
	if !strings.Contains(out, "to package.json") {
		t.Errorf("output = %q, want it to report the migration direction", out)
	}
}

func TestMigratePin_PackageJSONToTOML(t *testing.T) {
	dir := t.TempDir()
	pkgPath := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(`{"name":"app","driftr":{"node":"20.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := migratePin(dir, "node", "22.14.0", formatPackageJSON); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	cfg, err := config.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.Tools.GetTool("node") != "22.14.0" {
		t.Fatalf("%s does not carry the pin after migration: %+v", config.ProjectConfigFile, cfg)
	}
	// The driftr key must be dropped so the two files can't disagree.
	pkg, err := config.LoadPackageJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pkg != nil && pkg.Driftr.GetTool("node") != "" {
		t.Errorf("package.json still pins node after migration: %+v", pkg)
	}
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name"`) {
		t.Errorf("migration clobbered the rest of package.json: %q", data)
	}
	if !strings.Contains(out, "to "+config.ProjectConfigFile) {
		t.Errorf("output = %q, want it to report the migration direction", out)
	}
}

func TestMigratePin_NothingToMigrate(t *testing.T) {
	err := migratePin(t.TempDir(), "node", "22.14.0", formatNone)
	if err == nil || !strings.Contains(err.Error(), "no existing config to migrate") {
		t.Errorf("expected a no-config error, got: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "driftr pin node@") {
		t.Errorf("error = %v, want it to suggest the remediation command", err)
	}
}

func TestMigratePin_TOMLRemovalFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Claimed to be TOML-formatted, but the file isn't there: the pin lands in
	// package.json and the user must be told the stale file needs handling.
	captureStdout(t, func() {
		err := migratePin(dir, "node", "22.14.0", formatTOML)
		if err == nil {
			t.Fatal("expected an error when the TOML file cannot be removed")
		}
		if !strings.Contains(err.Error(), "Remove it manually") {
			t.Errorf("error = %v, want manual-removal guidance", err)
		}
	})

	pkg, err := config.LoadPackageJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pkg == nil || pkg.Driftr.GetTool("node") != "22.14.0" {
		t.Error("package.json should still have been written before the removal failed")
	}
}

func TestMigratePin_NoPackageJSONToMigrateInto(t *testing.T) {
	dir := t.TempDir()
	toml := filepath.Join(dir, config.ProjectConfigFile)
	if err := os.WriteFile(toml, []byte("[tools]\nnode = \"20.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := migratePin(dir, "node", "22.14.0", formatTOML)
	if err == nil || !strings.Contains(err.Error(), "no package.json found") {
		t.Fatalf("expected a missing-package.json error, got: %v", err)
	}
	// The existing pin must survive a migration that never started.
	if _, statErr := os.Stat(toml); statErr != nil {
		t.Errorf("%s was removed even though the migration failed", config.ProjectConfigFile)
	}
}

func TestPinCmd_MigrateWithoutConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")
	t.Chdir(t.TempDir())

	err := runCmd(t, "pin", "node@22.14.0", "--migrate")
	if err == nil || !strings.Contains(err.Error(), "no existing config to migrate") {
		t.Errorf("expected a no-config error, got: %v", err)
	}
}

func TestPinCmd_MigrateSwitchesFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, config.ProjectConfigFile), []byte("[tools]\nnode = \"22.14.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	if err := runCmd(t, "pin", "node@22.14.0", "--migrate"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, config.ProjectConfigFile)); !os.IsNotExist(err) {
		t.Errorf("%s survived --migrate", config.ProjectConfigFile)
	}
	pkg, err := config.LoadPackageJSON(proj)
	if err != nil {
		t.Fatal(err)
	}
	if pkg == nil || pkg.Driftr.GetTool("node") != "22.14.0" {
		t.Errorf("pin did not land in package.json: %+v", pkg)
	}
}

func TestPinCmd_UnknownTool(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	err := runCmd(t, "pin", "ruby@3.0.0")
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	if !strings.Contains(err.Error(), "ruby") {
		t.Errorf("error = %v, want it to name the unknown tool", err)
	}
}

func TestDetectPinFormat(t *testing.T) {
	t.Run("toml wins over package.json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, config.ProjectConfigFile), []byte("[tools]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"driftr":{"node":"20.0.0"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := detectPinFormat(dir); got != formatTOML {
			t.Errorf("detectPinFormat = %v, want formatTOML", got)
		}
	})

	t.Run("package.json without driftr key is not a format", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := detectPinFormat(dir); got != formatNone {
			t.Errorf("detectPinFormat = %v, want formatNone", got)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		if got := detectPinFormat(t.TempDir()); got != formatNone {
			t.Errorf("detectPinFormat = %v, want formatNone", got)
		}
	})
}
