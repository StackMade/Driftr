package installer

import (
	"archive/tar"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/version"
)

// registryFixture serves a fake npm registry for one package version: the
// version document, the package document, and the tarball itself.
type registryFixture struct {
	pkg       string
	ver       string
	tarball   []byte
	integrity string // dist.integrity as served; "" means the field is omitted
	host      string // tarball host override, for the poisoned-metadata case
	versions  []string
}

// newRegistryFixture builds a tarball whose only content is the package's
// binary, and the matching SRI integrity string.
func newRegistryFixture(t *testing.T, pkg, ver, binName string) *registryFixture {
	t.Helper()

	path := buildTarGz(t, []tarEntry{
		{Name: "package/package.json", Data: []byte(`{"name":"` + pkg + `"}`), Typeflag: tar.TypeReg},
		{Name: "package/bin/" + binName, Data: []byte("#!/usr/bin/env node\n"), Typeflag: tar.TypeReg},
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(data)

	return &registryFixture{
		pkg:       pkg,
		ver:       ver,
		tarball:   data,
		integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:]),
		versions:  []string{ver},
	}
}

// serve starts the fixture and points DRIFTR_NPM_REGISTRY at it.
func (f *registryFixture) serve(t *testing.T) {
	t.Helper()

	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tarballHost := base
		if f.host != "" {
			tarballHost = f.host
		}
		dist := map[string]any{"tarball": fmt.Sprintf("%s/%s/-/%s-%s.tgz", tarballHost, f.pkg, f.pkg, f.ver)}
		if f.integrity != "" {
			dist["integrity"] = f.integrity
		}

		switch {
		case strings.HasSuffix(r.URL.Path, ".tgz"):
			w.Write(f.tarball)
		case r.URL.Path == "/"+f.pkg:
			versions := map[string]any{}
			for _, v := range f.versions {
				versions[v] = map[string]any{"version": v, "dist": dist}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"name":      f.pkg,
				"dist-tags": map[string]string{"latest": f.versions[len(f.versions)-1]},
				"versions":  versions,
			})
		case r.URL.Path == "/"+f.pkg+"/"+f.ver:
			json.NewEncoder(w).Encode(map[string]any{
				"name": f.pkg, "version": f.ver, "dist": dist,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	base = srv.URL
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_NPM_REGISTRY", srv.URL)
}

func cachedTarball(t *testing.T, pkg, ver string) string {
	t.Helper()
	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tgz", pkg, ver))
}

func TestInstallPnpm_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs").serve(t)

	got, err := InstallPnpm("9.15.0", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "9.15.0" {
		t.Errorf("InstallPnpm() = %q, want %q", got, "9.15.0")
	}

	bin, err := platform.ToolBinary("pnpm", "9.15.0")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("pnpm binary not installed: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("pnpm binary is not executable: mode %v", info.Mode())
	}
}

func TestInstallPnpm_ResolvesPartialVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs")
	f.versions = []string{"8.15.0", "9.14.0", "9.15.0"}
	f.serve(t)

	got, err := InstallPnpm("9", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "9.15.0" {
		t.Errorf("InstallPnpm(9) = %q, want the newest 9.x %q", got, "9.15.0")
	}
}

// Registry metadata is attacker-controlled input: a tarball URL pointing at
// another host must be refused before anything is downloaded.
func TestInstallPnpm_RejectsForeignTarballHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs")
	f.host = "http://evil.example.com"
	f.serve(t)

	_, err := InstallPnpm("9.15.0", false)
	if err == nil || !strings.Contains(err.Error(), "unexpected host") {
		t.Fatalf("expected a host rejection, got: %v", err)
	}
	if _, statErr := os.Stat(cachedTarball(t, "pnpm", "9.15.0")); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a rejected tarball URL was downloaded anyway")
	}
}

// Without integrity data the package cannot be verified, so it must not be
// installed — and the unverified download must not stay in the cache.
func TestInstallPnpm_MissingIntegrityRefusesAndClearsCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs")
	f.integrity = ""
	f.serve(t)

	_, err := InstallPnpm("9.15.0", false)
	if err == nil || !strings.Contains(err.Error(), "missing integrity data") {
		t.Fatalf("expected a missing-integrity error, got: %v", err)
	}
	if _, statErr := os.Stat(cachedTarball(t, "pnpm", "9.15.0")); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("unverified archive left in the cache")
	}
	dir, err := platform.ToolVersionDir("pnpm", "9.15.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("unverified package was extracted")
	}
}

func TestInstallPnpm_IntegrityMismatchClearsCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs")
	f.integrity = "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))
	f.serve(t)

	_, err := InstallPnpm("9.15.0", false)
	if err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("expected an integrity mismatch, got: %v", err)
	}
	if _, statErr := os.Stat(cachedTarball(t, "pnpm", "9.15.0")); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("corrupt archive left in the cache")
	}
}

func TestInstallYarn_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	newRegistryFixture(t, "yarn", "1.22.22", "yarn.js").serve(t)

	got, err := InstallYarn("1.22.22", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.22.22" {
		t.Errorf("InstallYarn() = %q, want %q", got, "1.22.22")
	}
	bin, err := platform.ToolBinary("yarn", "1.22.22")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Errorf("yarn binary not installed: %v", err)
	}
}

func TestFetchRegistryVersion_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs").serve(t)

	_, err := FetchRegistryVersion("pnpm", "1.2.3")
	if err == nil || !strings.Contains(err.Error(), "not found in npm registry") {
		t.Fatalf("expected a not-found error, got: %v", err)
	}
}

func TestResolveRegistryLatest_NoMatchingVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs").serve(t)

	v, err := version.Parse("7")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveRegistryLatest("pnpm", v); err == nil ||
		!strings.Contains(err.Error(), "no pnpm version found") {
		t.Fatalf("expected a no-match error, got: %v", err)
	}
}

func TestResolveRegistryLatest_UsesDistTag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newRegistryFixture(t, "pnpm", "9.15.0", "pnpm.cjs")
	f.versions = []string{"8.15.0", "9.15.0"}
	f.serve(t)

	v, err := version.Parse("latest")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRegistryLatest("pnpm", v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "9.15.0" {
		t.Errorf("ResolveRegistryLatest(latest) = %q, want %q", got, "9.15.0")
	}
}
