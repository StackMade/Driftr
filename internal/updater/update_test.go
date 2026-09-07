package updater

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease serves the two endpoints Update talks to — the GitHub release
// index and the release assets — and points the updater at itself through the
// DRIFTR_UPDATE_API and DRIFTR_UPDATE_MIRROR overrides.
//
// Every case here stops before replaceBinary on purpose: past that point
// Update overwrites os.Executable(), which during a test run is the test
// binary itself. The successful replacement is covered by the e2e suite, which
// runs a second, throwaway binary.
func fakeRelease(t *testing.T, tag string, assets map[string][]byte) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			fmt.Fprintf(w, `{"tag_name":%q}`, tag)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v"+strings.TrimPrefix(tag, "v")+"/")
		data, ok := assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("DRIFTR_UPDATE_API", srv.URL)
	t.Setenv("DRIFTR_UPDATE_MIRROR", srv.URL)
}

func archiveName(version string) string {
	return fmt.Sprintf("driftr_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

func TestUpdate_AlreadyCurrent(t *testing.T) {
	fakeRelease(t, "v1.2.3", nil)

	version, err := Update("1.2.3", true)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if version != "" {
		t.Errorf("Update() = %q, want \"\" when already on the latest version", version)
	}
}

func TestUpdate_APIUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_UPDATE_API", srv.URL)

	_, err := Update("1.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "failed to check for updates") {
		t.Fatalf("Update() error = %v, want it to say the check failed", err)
	}
}

func TestUpdate_MissingAsset(t *testing.T) {
	// The release exists but carries no archive for this platform.
	fakeRelease(t, "v2.0.0", map[string][]byte{})

	_, err := Update("1.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("Update() error = %v, want a download failure", err)
	}
}

func TestUpdate_MissingChecksums(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"driftr": []byte("binary")})
	fakeRelease(t, "v2.0.0", map[string][]byte{archiveName("2.0.0"): archive})

	_, err := Update("1.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "failed to download checksums") {
		t.Fatalf("Update() error = %v, want a checksums download failure", err)
	}
}

// A tampered archive must never reach the extraction step.
func TestUpdate_ChecksumMismatch(t *testing.T) {
	name := archiveName("2.0.0")
	archive := makeTarGz(t, map[string][]byte{"driftr": []byte("binary")})
	checksums := []byte(sha256Hex([]byte("a different archive")) + "  " + name + "\n")

	fakeRelease(t, "v2.0.0", map[string][]byte{name: archive, "checksums.txt": checksums})

	_, err := Update("1.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Update() error = %v, want a checksum mismatch", err)
	}
}

func TestUpdate_ArchiveWithoutBinary(t *testing.T) {
	name := archiveName("2.0.0")
	archive := makeTarGz(t, map[string][]byte{"README.md": []byte("no binary here")})
	checksums := []byte(sha256Hex(archive) + "  " + name + "\n")

	fakeRelease(t, "v2.0.0", map[string][]byte{name: archive, "checksums.txt": checksums})

	_, err := Update("1.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "extraction failed") {
		t.Fatalf("Update() error = %v, want an extraction failure", err)
	}
}
