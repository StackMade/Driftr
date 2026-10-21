package installer

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackmade/driftr/v2/internal/version"
)

func TestParseSRI_Valid(t *testing.T) {
	rawHash := make([]byte, 64) // 512 bits = 64 bytes
	rawHash[0] = 0xAB
	rawHash[63] = 0xCD
	b64 := base64.StdEncoding.EncodeToString(rawHash)
	sri := "sha512-" + b64

	algo, hash, err := parseSRI(sri)
	if err != nil {
		t.Fatalf("parseSRI() error: %v", err)
	}
	if algo != "sha512" {
		t.Errorf("algo = %q, want %q", algo, "sha512")
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
	if hash[0] != 0xAB || hash[63] != 0xCD {
		t.Error("hash content mismatch")
	}
}

func TestParseSRI_Invalid(t *testing.T) {
	tests := []string{
		"",
		"sha512",
		"nohyphen",
		"sha512-!!!invalid-base64!!!",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, _, err := parseSRI(input)
			if err == nil {
				t.Errorf("parseSRI(%q) expected error, got nil", input)
			}
		})
	}
}

func TestVerifyIntegrity_Valid(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello npm registry")
	path := filepath.Join(dir, "test.tgz")

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	h := sha512.Sum512(content)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(h[:])

	if err := VerifyIntegrity(path, integrity); err != nil {
		t.Fatalf("VerifyIntegrity() error: %v", err)
	}
}

func TestVerifyIntegrity_Mismatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello npm registry")
	path := filepath.Join(dir, "test.tgz")

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	wrongHash := make([]byte, 64)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(wrongHash)

	err := VerifyIntegrity(path, integrity)
	if err == nil {
		t.Fatal("expected integrity mismatch error, got nil")
	}
}

func TestVerifyIntegrity_UnsupportedAlgo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tgz")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := VerifyIntegrity(path, "sha256-AAAA")
	if err == nil {
		t.Fatal("expected unsupported algorithm error, got nil")
	}
}

func TestListRemoteVersions(t *testing.T) {
	pkg := registryPackage{
		Name:     "pnpm",
		DistTags: map[string]string{"latest": "9.1.0"},
		Versions: map[string]registryVersion{
			"9.1.0":      {Version: "9.1.0"},
			"9.0.0":      {Version: "9.0.0"},
			"8.15.3":     {Version: "8.15.3"},
			"9.2.0-rc.0": {Version: "9.2.0-rc.0"},
			"9.1.0-rc.1": {Version: "9.1.0-rc.1"},
			"bad-ver":    {Version: "bad-ver"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(pkg)
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = &http.Client{Transport: &hostRewriteTransport{target: srv.URL}}
	defer func() { httpClient = orig }()

	t.Run("stable only", func(t *testing.T) {
		versions, err := ListRemoteVersions("pnpm", false)
		if err != nil {
			t.Fatalf("ListRemoteVersions() error: %v", err)
		}
		if len(versions) != 3 {
			t.Fatalf("expected 3 stable versions, got %d: %v", len(versions), versions)
		}
		if versions[0] != "9.1.0" {
			t.Errorf("versions[0] = %q, want 9.1.0", versions[0])
		}
		if versions[1] != "9.0.0" {
			t.Errorf("versions[1] = %q, want 9.0.0", versions[1])
		}
		if versions[2] != "8.15.3" {
			t.Errorf("versions[2] = %q, want 8.15.3", versions[2])
		}
	})

	t.Run("include pre-release", func(t *testing.T) {
		versions, err := ListRemoteVersions("pnpm", true)
		if err != nil {
			t.Fatalf("ListRemoteVersions() error: %v", err)
		}
		// bad-ver cannot be parsed, so 5 valid versions (3 stable + 2 rc).
		if len(versions) != 5 {
			t.Fatalf("expected 5 versions, got %d: %v", len(versions), versions)
		}
		// rc should be first since 9.2.0 > 9.1.0, and 9.1.0 beats its own rc.
		want := []string{"9.2.0-rc.0", "9.1.0", "9.1.0-rc.1", "9.0.0", "8.15.3"}
		for i, w := range want {
			if versions[i] != w {
				t.Errorf("versions = %v, want %v", versions, want)
				break
			}
		}
	})
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int // positive if a > b, negative if a < b, 0 if equal
	}{
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, err := version.Parse(tt.a)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.a, err)
			}
			b, err := version.Parse(tt.b)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.b, err)
			}
			if got := versionCompare(a, b); got != tt.want {
				t.Errorf("versionCompare(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestValidateTarballURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid registry URL", "https://registry.npmjs.org/pnpm/-/pnpm-9.1.0.tgz", false},
		{"mixed-case host accepted", "https://Registry.NPMJS.org/pnpm/-/pnpm-9.1.0.tgz", false},
		{"plain HTTP rejected", "http://registry.npmjs.org/pnpm/-/pnpm-9.1.0.tgz", true},
		{"wrong host rejected", "https://evil.example.com/pnpm-9.1.0.tgz", true},
		{"host with port rejected", "https://registry.npmjs.org:8443/pnpm.tgz", true},
		{"unparseable URL rejected", "https://registry.npmjs.org/%zz", true},
		{"empty URL rejected", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTarballURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTarballURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestRegistryBase_EnvOverride(t *testing.T) {
	t.Setenv("DRIFTR_NPM_REGISTRY", "http://127.0.0.1:9123/")
	if got := registryBase(); got != "http://127.0.0.1:9123" {
		t.Errorf("registryBase() = %q, want trailing slash trimmed", got)
	}
}

func TestValidateTarballURL_CustomRegistry(t *testing.T) {
	t.Setenv("DRIFTR_NPM_REGISTRY", "http://127.0.0.1:9123")

	if err := validateTarballURL("http://127.0.0.1:9123/pnpm/-/pnpm-9.1.0.tgz"); err != nil {
		t.Errorf("tarball URL matching custom registry rejected: %v", err)
	}
	if err := validateTarballURL("https://registry.npmjs.org/pnpm/-/pnpm-9.1.0.tgz"); err == nil {
		t.Error("tarball URL from a different host than the configured registry must be rejected")
	}
}
