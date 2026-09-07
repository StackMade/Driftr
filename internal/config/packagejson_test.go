package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writePackageJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test package.json: %v", err)
	}
}

func TestLoadPackageJSON_DriftrNode(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "driftr": {"node": "20.11.0"}}`)

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg == nil {
		t.Fatal("expected non-nil result")
	}
	if pkg.Driftr.GetTool("node") != "20.11.0" {
		t.Errorf("got %q, want %q", pkg.Driftr.GetTool("node"), "20.11.0")
	}
}

func TestLoadPackageJSON_PackageManagerOnly(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "packageManager": "pnpm@9.15.0"}`)

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg == nil {
		t.Fatal("expected non-nil result when only packageManager is set")
	}
	tool, ver := pkg.PackageManagerTool()
	if tool != "pnpm" || ver != "9.15.0" {
		t.Errorf("PackageManagerTool() = (%q, %q), want (pnpm, 9.15.0)", tool, ver)
	}
}

func TestPackageManagerTool(t *testing.T) {
	tests := []struct {
		field    string
		wantTool string
		wantVer  string
	}{
		{"pnpm@9.15.0", "pnpm", "9.15.0"},
		{"yarn@1.22.22", "yarn", "1.22.22"},
		{"yarn@1.22.22+sha512.abcdef", "yarn", "1.22.22"},
		{"", "", ""},
		{"pnpm", "", ""},
		{"pnpm@", "", ""},
		{"@9.15.0", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			pkg := &PackageJSON{PackageManager: tt.field}
			tool, ver := pkg.PackageManagerTool()
			if tool != tt.wantTool || ver != tt.wantVer {
				t.Errorf("PackageManagerTool() = (%q, %q), want (%q, %q)", tool, ver, tt.wantTool, tt.wantVer)
			}
		})
	}
}

func TestLoadPackageJSON_NoDriftr(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "version": "1.0.0"}`)

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg != nil {
		t.Errorf("expected nil for package.json without driftr, got %+v", pkg)
	}
}

func TestLoadPackageJSON_EmptyDriftrNode(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"driftr": {"node": ""}}`)

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg != nil {
		t.Errorf("expected nil for empty driftr.node, got %+v", pkg)
	}
}

func TestLoadPackageJSON_NoFile(t *testing.T) {
	dir := t.TempDir()

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg != nil {
		t.Errorf("expected nil when no package.json exists, got %+v", pkg)
	}
}

func TestLoadPackageJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, "{invalid")

	_, err := LoadPackageJSON(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSavePackageJSON_AddsKey(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "version": "1.0.0"}`)

	if err := SavePackageJSON(dir, "22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON after save: %v", err)
	}

	// Verify driftr key was added.
	driftrRaw, ok := raw["driftr"]
	if !ok {
		t.Fatal("driftr key missing from package.json")
	}
	var dc DriftrConfig
	if err := json.Unmarshal(driftrRaw, &dc); err != nil {
		t.Fatalf("failed to unmarshal driftr config: %v", err)
	}
	if dc.GetTool("node") != "22.14.0" {
		t.Errorf("got %q, want %q", dc.GetTool("node"), "22.14.0")
	}

	// Verify existing keys are preserved.
	if _, ok := raw["name"]; !ok {
		t.Error("name key was lost")
	}
	if _, ok := raw["version"]; !ok {
		t.Error("version key was lost")
	}
}

func TestSavePackageJSON_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "driftr": {"node": "20.0.0"}}`)

	if err := SavePackageJSON(dir, "22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg.Driftr.GetTool("node") != "22.14.0" {
		t.Errorf("got %q, want %q", pkg.Driftr.GetTool("node"), "22.14.0")
	}
}

func TestSavePackageJSON_NoFile(t *testing.T) {
	dir := t.TempDir()

	err := SavePackageJSON(dir, "22.14.0")
	if err == nil {
		t.Fatal("expected error when package.json doesn't exist")
	}
}

func TestRemoveDriftrFromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "driftr": {"node": "20.0.0"}}`)

	if err := RemoveDriftrFromPackageJSON(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if _, ok := raw["driftr"]; ok {
		t.Error("driftr key should have been removed")
	}
	if _, ok := raw["name"]; !ok {
		t.Error("name key was lost")
	}
}

func TestRemoveDriftrFromPackageJSON_NoFile(t *testing.T) {
	dir := t.TempDir()

	if err := RemoveDriftrFromPackageJSON(dir); err != nil {
		t.Fatalf("expected nil error when no package.json, got: %v", err)
	}
}

func TestRemoveDriftrFromPackageJSON_NoDriftrKey(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp"}`)

	if err := RemoveDriftrFromPackageJSON(dir); err != nil {
		t.Fatalf("expected nil error when no driftr key, got: %v", err)
	}
}

func readPackageJSONBytes(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("failed to read package.json: %v", err)
	}
	return data
}

func TestSavePackageJSON_PreservesKeyOrder(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{
  "name": "myapp",
  "version": "1.0.0",
  "scripts": {"test": "jest"}
}
`)

	if err := SavePackageJSON(dir, "22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := readPackageJSONBytes(t, dir)

	nameIdx := bytes.Index(data, []byte(`"name"`))
	versionIdx := bytes.Index(data, []byte(`"version"`))
	scriptsIdx := bytes.Index(data, []byte(`"scripts"`))
	driftrIdx := bytes.Index(data, []byte(`"driftr"`))

	if nameIdx > versionIdx || versionIdx > scriptsIdx || scriptsIdx > driftrIdx {
		t.Errorf("key order not preserved, want name < version < scripts < driftr:\n%s", data)
	}
}

func TestSavePackageJSON_UpdatePreservesPosition(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{
  "name": "myapp",
  "driftr": {"node": "20.0.0"},
  "version": "1.0.0"
}
`)

	if err := SavePackageJSON(dir, "22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := readPackageJSONBytes(t, dir)

	nameIdx := bytes.Index(data, []byte(`"name"`))
	driftrIdx := bytes.Index(data, []byte(`"driftr"`))
	versionIdx := bytes.Index(data, []byte(`"version"`))

	if nameIdx > driftrIdx || driftrIdx > versionIdx {
		t.Errorf("key order not preserved, want name < driftr < version:\n%s", data)
	}
}

func TestRemoveDriftrFromPackageJSON_PreservesKeyOrder(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{
  "name": "myapp",
  "driftr": {"node": "20.0.0"},
  "version": "1.0.0",
  "scripts": {}
}
`)

	if err := RemoveDriftrFromPackageJSON(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := readPackageJSONBytes(t, dir)

	if bytes.Contains(data, []byte(`"driftr"`)) {
		t.Error("driftr key should have been removed")
	}

	nameIdx := bytes.Index(data, []byte(`"name"`))
	versionIdx := bytes.Index(data, []byte(`"version"`))
	scriptsIdx := bytes.Index(data, []byte(`"scripts"`))

	if nameIdx > versionIdx || versionIdx > scriptsIdx {
		t.Errorf("key order not preserved, want name < version < scripts:\n%s", data)
	}
}

func TestLoadPackageJSON_PnpmAndYarn(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "driftr": {"node": "22.14.0", "pnpm": "9.15.0", "yarn": "1.22.22"}}`)

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg == nil {
		t.Fatal("expected non-nil result")
	}
	if pkg.Driftr.GetTool("node") != "22.14.0" {
		t.Errorf("node = %q, want %q", pkg.Driftr.GetTool("node"), "22.14.0")
	}
	if pkg.Driftr.GetTool("pnpm") != "9.15.0" {
		t.Errorf("pnpm = %q, want %q", pkg.Driftr.GetTool("pnpm"), "9.15.0")
	}
	if pkg.Driftr.GetTool("yarn") != "1.22.22" {
		t.Errorf("yarn = %q, want %q", pkg.Driftr.GetTool("yarn"), "1.22.22")
	}
}

func TestLoadPackageJSON_PnpmOnly(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "driftr": {"pnpm": "9.15.0"}}`)

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg == nil {
		t.Fatal("expected non-nil result for pnpm-only config")
	}
	if pkg.Driftr.GetTool("pnpm") != "9.15.0" {
		t.Errorf("pnpm = %q, want %q", pkg.Driftr.GetTool("pnpm"), "9.15.0")
	}
	if pkg.Driftr.GetTool("node") != "" {
		t.Errorf("node should be empty, got %q", pkg.Driftr.GetTool("node"))
	}
}

func TestSavePackageJSONTool_Pnpm(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp", "driftr": {"node": "22.14.0"}}`)

	if err := SavePackageJSONTool(dir, "pnpm", "9.15.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg.Driftr.GetTool("node") != "22.14.0" {
		t.Errorf("node should be preserved, got %q", pkg.Driftr.GetTool("node"))
	}
	if pkg.Driftr.GetTool("pnpm") != "9.15.0" {
		t.Errorf("pnpm = %q, want %q", pkg.Driftr.GetTool("pnpm"), "9.15.0")
	}
}

func TestSavePackageJSONTool_Yarn(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "myapp"}`)

	if err := SavePackageJSONTool(dir, "yarn", "1.22.22"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkg, err := LoadPackageJSON(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg.Driftr.GetTool("yarn") != "1.22.22" {
		t.Errorf("yarn = %q, want %q", pkg.Driftr.GetTool("yarn"), "1.22.22")
	}
}

func TestDriftrConfig_SetAndGet(t *testing.T) {
	var dc DriftrConfig
	dc.SetTool("node", "22")
	dc.SetTool("pnpm", "9")
	dc.SetTool("yarn", "1")

	if dc.GetTool("node") != "22" {
		t.Errorf("node = %q, want %q", dc.GetTool("node"), "22")
	}
	if dc.GetTool("pnpm") != "9" {
		t.Errorf("pnpm = %q, want %q", dc.GetTool("pnpm"), "9")
	}
	if dc.GetTool("yarn") != "1" {
		t.Errorf("yarn = %q, want %q", dc.GetTool("yarn"), "1")
	}
}

func TestDriftrConfig_GetTool_Unknown(t *testing.T) {
	dc := DriftrConfig{"node": "22", "pnpm": "9", "yarn": "1"}
	if got := dc.GetTool("bun"); got != "" {
		t.Errorf("GetTool(\"bun\") = %q, want empty string", got)
	}
}

func TestSavePackageJSON_PreservesIndentation(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{
    "name": "myapp",
    "version": "1.0.0"
}
`)

	if err := SavePackageJSON(dir, "22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := readPackageJSONBytes(t, dir)

	// Should detect 4-space indent from the original file.
	if !bytes.Contains(data, []byte("    \"name\"")) {
		t.Errorf("expected 4-space indentation to be preserved:\n%s", data)
	}
	if !bytes.Contains(data, []byte("    \"driftr\"")) {
		t.Errorf("expected driftr key to use same 4-space indentation:\n%s", data)
	}
}

func TestSavePackageJSONTool_MalformedDriftrKey(t *testing.T) {
	dir := t.TempDir()
	original := `{"name": "myapp", "driftr": {"node": 22, "pnpm": "9.15.0"}}`
	writePackageJSON(t, dir, original)

	err := SavePackageJSONTool(dir, "yarn", "4.5.0")
	if err == nil {
		t.Fatal("expected error for malformed driftr key, got nil")
	}

	// The file must be left untouched so no pinned tools are lost.
	data, readErr := os.ReadFile(filepath.Join(dir, "package.json"))
	if readErr != nil {
		t.Fatalf("unexpected error: %v", readErr)
	}
	if string(data) != original {
		t.Errorf("package.json was modified despite parse error:\ngot:  %s\nwant: %s", data, original)
	}
}

func TestLoadPackageJSON_EnginesNode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantNil bool
	}{
		{"range only", `{"name":"app","engines":{"node":">=18"}}`, ">=18", false},
		{"trimmed", `{"engines":{"node":"  ^20.9.0  "}}`, "^20.9.0", false},
		{"alongside driftr key", `{"driftr":{"node":"20.11.0"},"engines":{"node":">=18"}}`, ">=18", false},
		{"other engines only", `{"engines":{"npm":">=9"}}`, "", true},
		{"empty node", `{"engines":{"node":""}}`, "", true},
		{"no engines", `{"name":"app"}`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writePackageJSON(t, dir, tt.content)

			pkg, err := LoadPackageJSON(dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if pkg != nil {
					t.Fatalf("expected nil result, got %+v", pkg)
				}
				return
			}
			if pkg == nil {
				t.Fatal("expected non-nil result")
			}
			if got := pkg.EnginesNode(); got != tt.want {
				t.Errorf("EnginesNode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnginesNode_NilReceiver(t *testing.T) {
	var pkg *PackageJSON
	if got := pkg.EnginesNode(); got != "" {
		t.Errorf("EnginesNode() = %q, want empty", got)
	}
}
