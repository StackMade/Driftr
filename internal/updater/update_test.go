package updater

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// Cases stop before replaceBinary: past it Update overwrites os.Executable(), the test binary.
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
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("driftr_%s_%s_%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)
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

func TestUpdate_NeverDowngrades(t *testing.T) {
	fakeRelease(t, "v1.2.3", nil)

	version, err := Update("1.10.0", false)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if version != "" {
		t.Errorf("Update() = %q, want \"\" when the running build is newer than the release", version)
	}
}

func TestUpdate_RefusesDevBuild(t *testing.T) {
	fakeRelease(t, "v1.2.3", nil)

	_, err := Update("dev", false)
	if err == nil || !strings.Contains(err.Error(), "not a release build") || !strings.Contains(err.Error(), "install.sh") {
		t.Fatalf("Update() error = %v, want a refusal naming the installer", err)
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

// fakeReleases serves a stable release at /releases/latest and the given tags
// as the release list, and records which archive Update asks for.
func fakeReleases(t *testing.T, latest string, list ...string) *string {
	t.Helper()

	var requested string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q}`, latest)
		case "/releases":
			items := make([]string, 0, len(list))
			for _, tag := range list {
				items = append(items, fmt.Sprintf(`{"tag_name":%q,"draft":false}`, tag))
			}
			fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
		default:
			requested = r.URL.Path
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("DRIFTR_UPDATE_API", srv.URL)
	t.Setenv("DRIFTR_UPDATE_MIRROR", srv.URL)
	return &requested
}

func TestUpdate_PreReleaseFollowsPreReleases(t *testing.T) {
	requested := fakeReleases(t, "v1.5.2", "nightly", "v1.5.2", "v2.0.0-rc.2", "v2.0.0-rc.1")

	_, err := Update("2.0.0-rc.1", false)
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("Update() error = %v, want a download failure", err)
	}
	if want := "/v2.0.0-rc.2/" + archiveName("2.0.0-rc.2"); *requested != want {
		t.Errorf("Update() fetched %q, want %q", *requested, want)
	}
}

func TestUpdate_StableIgnoresPreReleases(t *testing.T) {
	fakeReleases(t, "v1.5.2", "v2.0.0-rc.1", "v1.5.2")

	version, err := Update("1.5.2", false)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if version != "" {
		t.Errorf("Update() = %q, want \"\": a stable build never moves to a pre-release", version)
	}
}

func TestUpdate_RefusesNightlyBuild(t *testing.T) {
	fakeReleases(t, "v1.5.2")

	_, err := Update("1.5.1-SNAPSHOT-abc1234", false)
	if err == nil || !strings.Contains(err.Error(), "not a release build") {
		t.Fatalf("Update() error = %v, want a refusal", err)
	}
}
