package nodeenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPackageManager(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  PackageManager
	}{
		{
			name:  "packageManager field wins over lockfile",
			files: map[string]string{"package.json": `{"packageManager":"pnpm@9.1.0"}`, "yarn.lock": ""},
			want:  PnpmManager,
		},
		{
			name:  "yarn from packageManager field",
			files: map[string]string{"package.json": `{"packageManager":"yarn@4.0.0"}`},
			want:  YarnManager,
		},
		{
			name:  "npm from packageManager field",
			files: map[string]string{"package.json": `{"packageManager":"npm@10.0.0"}`},
			want:  NpmManager,
		},
		{
			name:  "pnpm from lockfile",
			files: map[string]string{"pnpm-lock.yaml": ""},
			want:  PnpmManager,
		},
		{
			name:  "yarn from lockfile",
			files: map[string]string{"yarn.lock": ""},
			want:  YarnManager,
		},
		{
			name:  "npm from lockfile",
			files: map[string]string{"package-lock.json": ""},
			want:  NpmManager,
		},
		{
			name:  "lockfile fallthrough when packageManager absent",
			files: map[string]string{"package.json": `{"name":"app"}`, "pnpm-lock.yaml": ""},
			want:  PnpmManager,
		},
		{
			name:  "nothing detected",
			files: map[string]string{"package.json": `{"name":"app"}`},
			want:  NoManager,
		},
		{
			name:  "empty dir",
			files: map[string]string{},
			want:  NoManager,
		},
		{
			name:  "malformed package.json falls through",
			files: map[string]string{"package.json": `{not json`, "package-lock.json": ""},
			want:  NpmManager,
		},
		{
			name:  "unknown packageManager name falls through to lockfile",
			files: map[string]string{"package.json": `{"packageManager":"bun@1.1.0"}`, "yarn.lock": ""},
			want:  YarnManager,
		},
		{
			name:  "packageManager without version suffix",
			files: map[string]string{"package.json": `{"packageManager":"pnpm"}`},
			want:  PnpmManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := DetectPackageManager(dir); got != tt.want {
				t.Errorf("DetectPackageManager = %q, want %q", got, tt.want)
			}
		})
	}
}

// A directory named like a lockfile is not a lockfile.
func TestDetectPackageManager_LockfileDirIsNotALockfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pnpm-lock.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := DetectPackageManager(dir); got != NoManager {
		t.Errorf("DetectPackageManager = %q, want %q", got, NoManager)
	}
}

// A directory in place of package.json must not abort detection.
func TestDetectPackageManager_PackageJSONDirFallsThrough(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "package.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yarn.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectPackageManager(dir); got != YarnManager {
		t.Errorf("DetectPackageManager = %q, want %q", got, YarnManager)
	}
}
