package ioutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyFile_CopiesContentAndMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	want := []byte("#!/bin/sh\necho hello\n")
	if err := os.WriteFile(src, want, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("copied content = %q, want %q", got, want)
	}

	// The destination is a binary the updater is about to run, so it has to
	// come out executable regardless of the source's mode.
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("destination mode = %v, want it executable", info.Mode())
	}
}

func TestCopyFile_TruncatesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(strings.Repeat("x", 100)), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "short" {
		t.Errorf("destination = %q, want the source content with no leftovers", got)
	}
}

func TestCopyFile_Errors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("source missing", func(t *testing.T) {
		if err := CopyFile(filepath.Join(dir, "absent"), filepath.Join(dir, "out")); err == nil {
			t.Fatal("expected an error for a missing source")
		}
	})

	t.Run("destination not creatable", func(t *testing.T) {
		// A path under a file rather than a directory cannot be created.
		if err := CopyFile(src, filepath.Join(src, "nested")); err == nil {
			t.Fatal("expected an error for an uncreatable destination")
		}
	})
}
