package installer

import (
	"archive/tar"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stackmade/driftr/v2/internal/platform"
)

// Racing installs of one version must all succeed: each unpacks to a temp dir, then renames.
func concurrentExtractors(t *testing.T, n int, extract func() error) {
	t.Helper()

	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // widen the window in which the renames collide
			errs[i] = extract()
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("extractor %d failed: %v", i, err)
		}
	}
}

func assertSoleInstall(t *testing.T, destDir, binPath, wantContent string) {
	t.Helper()

	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("binary missing after concurrent extraction: %v", err)
	}
	if !platform.IsExecutable(info) {
		t.Errorf("binary is not executable: mode %v", info.Mode())
	}
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wantContent {
		t.Errorf("binary content = %q, want %q", got, wantContent)
	}

	entries, err := os.ReadDir(filepath.Dir(destDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(destDir) {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected exactly one install directory %q, got %v",
			filepath.Base(destDir), names)
	}
}

func TestExtractRegistryPackage_ConcurrentInstalls(t *testing.T) {
	const body = "#!/usr/bin/env node\nconsole.log(1)\n"
	archive := buildTarGz(t, []tarEntry{
		{Name: "package/package.json", Data: []byte(`{"name":"pnpm"}`), Typeflag: tar.TypeReg},
		{Name: "package/bin/pnpm.cjs", Data: []byte(body), Typeflag: tar.TypeReg},
	})

	destDir := filepath.Join(t.TempDir(), "9.15.0")
	binPath := filepath.Join(destDir, "bin", "pnpm.cjs")

	concurrentExtractors(t, 8, func() error {
		return ExtractRegistryPackage(archive, destDir, binPath)
	})

	assertSoleInstall(t, destDir, binPath, body)
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Errorf("package.json missing from the surviving install: %v", err)
	}
}

func TestExtractZipToBin_ConcurrentInstalls(t *testing.T) {
	const body = "#!/bin/sh\necho bun\n"
	archive := writeTestZip(t, map[string]string{
		"bun-linux-x64/bun":     body,
		"bun-linux-x64/LICENSE": "MIT",
	})

	destDir := filepath.Join(t.TempDir(), "1.2.3")
	binPath := filepath.Join(destDir, "bin", "bun")

	concurrentExtractors(t, 8, func() error {
		return ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
	})

	assertSoleInstall(t, destDir, binPath, body)
}

// A version dir without its binary, left by a removal or install that stopped
// halfway, is replaced rather than blocking every later install.
func TestExtractZipToBin_ReplacesDirWithoutBinary(t *testing.T) {
	archive := writeTestZip(t, map[string]string{"bun-linux-x64/bun": "#!/bin/sh\n"})

	parent := t.TempDir()
	destDir := filepath.Join(parent, "1.2.3")
	if err := os.MkdirAll(filepath.Join(destDir, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun"); err != nil {
		t.Fatalf("ExtractZipToBin() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "bin", "bun")); err != nil {
		t.Errorf("binary missing after the install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "occupied")); !os.IsNotExist(err) {
		t.Errorf("the incomplete dir's content survived: %v", err)
	}
}
