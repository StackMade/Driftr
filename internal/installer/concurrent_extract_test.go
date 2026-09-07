package installer

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// concurrentExtractors runs extract N times at once and fails the test on the
// first error any of them reports. Both extractors claim to survive two
// installs of the same version racing each other: they unpack into a unique
// sibling temp dir and rename it into place, treating a rename failure as
// "another process got there first" once the binary exists.
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

// assertSoleInstall checks that destDir is the only thing its parent holds —
// no abandoned ".tmp-" work dir — and that the named binary is there, has the
// expected content, and is executable.
func assertSoleInstall(t *testing.T, destDir, binPath, wantContent string) {
	t.Helper()

	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("binary missing after concurrent extraction: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
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
		var names []string
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

// A loser of the race must not report success while the destination is
// unusable: with the winner's directory present but empty, the rename fails
// and the binary is nowhere, so the error has to surface.
func TestExtractZipToBin_RenameFailureWithoutBinaryErrors(t *testing.T) {
	archive := writeTestZip(t, map[string]string{"bun-linux-x64/bun": "#!/bin/sh\n"})

	parent := t.TempDir()
	destDir := filepath.Join(parent, "1.2.3")
	// A directory that is occupied but holds no binary: renaming onto it fails
	// and the "someone else won" check must not paper over that.
	if err := os.MkdirAll(filepath.Join(destDir, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
	if err == nil || !strings.Contains(err.Error(), "finalize install") {
		t.Fatalf("expected a finalize-install error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "bin", "bun")); statErr == nil {
		t.Error("failed install left a binary behind")
	}
}
