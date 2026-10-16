package shim

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

func TestWriteShim_Content(t *testing.T) {
	dir := t.TempDir()
	driftrBin := "/usr/local/bin/driftr"

	if err := writeShim(dir, "node", driftrBin); err != nil {
		t.Fatalf("writeShim() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "node"))
	if err != nil {
		t.Fatalf("failed to read shim: %v", err)
	}

	shim := string(content)
	if !strings.HasPrefix(shim, "#!/bin/sh\n") {
		t.Error("shim should start with #!/bin/sh")
	}
	if !strings.Contains(shim, `exec '/usr/local/bin/driftr' shim node "$@"`) {
		t.Errorf("shim content unexpected: %s", shim)
	}
}

func TestWriteShim_Executable(t *testing.T) {
	dir := t.TempDir()

	if err := writeShim(dir, "pnpm", "/usr/local/bin/driftr"); err != nil {
		t.Fatalf("writeShim() error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "pnpm"))
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("shim should be executable")
	}
}

func TestWriteShim_RestoresExecutableBit(t *testing.T) {
	dir := t.TempDir()
	shimPath := filepath.Join(dir, "node")

	if err := writeShim(dir, "node", "/usr/local/bin/driftr"); err != nil {
		t.Fatalf("writeShim() error: %v", err)
	}
	if err := os.Chmod(shimPath, 0o644); err != nil {
		t.Fatal(err)
	}

	// os.WriteFile sets the mode only on create, so rewriting a shim must chmod it explicitly.
	if err := writeShim(dir, "node", "/usr/local/bin/driftr"); err != nil {
		t.Fatalf("writeShim() error on rewrite: %v", err)
	}

	info, err := os.Stat(shimPath)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("rewritten shim is not executable: %v", info.Mode())
	}
}

func TestWriteShim_AllTools(t *testing.T) {
	dir := t.TempDir()

	for _, tool := range ShimTools() {
		if err := writeShim(dir, tool, "/bin/driftr"); err != nil {
			t.Fatalf("writeShim(%q) error: %v", tool, err)
		}
		if _, err := os.Stat(filepath.Join(dir, tool)); errors.Is(err, os.ErrNotExist) {
			t.Errorf("shim for %q was not created", tool)
		}
	}

	expected := map[string]bool{
		"node": true, "npm": true, "npx": true,
		"pnpm": true, "pnpx": true, "yarn": true,
		"bun": true,
	}
	for _, tool := range ShimTools() {
		if !expected[tool] {
			t.Errorf("unexpected tool in ShimTools(): %q", tool)
		}
		delete(expected, tool)
	}
	for tool := range expected {
		t.Errorf("missing tool in ShimTools(): %q", tool)
	}
}

func TestGenerateShims(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := GenerateShims("test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	binDir, err := ShimDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range ShimTools() {
		shimPath := filepath.Join(binDir, tool)
		info, err := os.Stat(shimPath)
		if err != nil {
			t.Errorf("shim for %s not created: %v", tool, err)
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("shim for %s is not executable: %v", tool, info.Mode())
		}
		content, err := os.ReadFile(shimPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "shim "+tool) {
			t.Errorf("shim for %s has wrong invocation: %s", tool, content)
		}
	}
}

func TestShimTools_ReturnsCopy(t *testing.T) {
	tools := ShimTools()
	if len(tools) == 0 {
		t.Fatal("expected non-empty tool list")
	}
	tools[0] = "mutated"
	if ShimTools()[0] == "mutated" {
		t.Error("ShimTools returns a reference to the internal slice; callers can mutate it")
	}
}

func TestGenerateShims_PackageBins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// The developer's own tsc must not count as "already on PATH".
	t.Setenv("PATH", t.TempDir())
	binDir, _ := ShimDir()
	nodeDir, _ := platform.NodeVersionDir("22.0.0")
	if err := os.MkdirAll(filepath.Join(nodeDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeDir, "bin", "tsc"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A stale shim from an older driftr path goes; a file that only looks similar stays.
	stale := "#!/bin/sh\nexec \"/old/driftr\" shim gone \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "gone"), []byte(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "#!/bin/sh\nexec \"/old/driftr\" shim other \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "mine"), []byte(mine), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := GenerateShims("test"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(binDir, "tsc")); err != nil {
		t.Errorf("tsc shim not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "gone")); !os.IsNotExist(err) {
		t.Errorf("stale shim kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "mine")); err != nil {
		t.Errorf("non-matching file removed: %v", err)
	}
}

func TestGenerateShims_NeverOverwritesAForeignFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	binDir, _ := ShimDir()
	nodeDir, _ := platform.NodeVersionDir("22.0.0")
	if err := os.MkdirAll(filepath.Join(nodeDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A package bin named like the driftr binary the user keeps in the bin dir.
	if err := os.WriteFile(filepath.Join(nodeDir, "bin", "driftr"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userBinary := []byte("\x7fELF not a shim")
	if err := os.WriteFile(filepath.Join(binDir, "driftr"), userBinary, 0o755); err != nil {
		t.Fatal(err)
	}

	skipped, err := GenerateShims("test")
	if err != nil {
		t.Fatal(err)
	}

	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "driftr: ") {
		t.Errorf("want driftr reported as skipped, got %q", skipped)
	}
	if got, _ := os.ReadFile(filepath.Join(binDir, "driftr")); string(got) != string(userBinary) {
		t.Errorf("file in the bin dir was overwritten: %q", got)
	}
}

func TestRefreshIfStale(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	binDir, _ := ShimDir()

	// Before setup there is no shim dir, and a refresh must not create one.
	if err := RefreshIfStale("1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binDir); !os.IsNotExist(err) {
		t.Fatalf("refresh created the shim dir before setup: %v", err)
	}

	// Shims from a driftr that predates the stamp: one refresh brings them up.
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RefreshIfStale("1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "node")); err != nil {
		t.Fatalf("stale shims were not regenerated: %v", err)
	}

	// Same version again: nothing is rewritten.
	if err := os.Remove(filepath.Join(binDir, "node")); err != nil {
		t.Fatal(err)
	}
	if err := RefreshIfStale("1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "node")); !os.IsNotExist(err) {
		t.Errorf("current shims were regenerated anyway: %v", err)
	}

	// An upgrade: a different version regenerates once more.
	if err := RefreshIfStale("1.1.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "node")); err != nil {
		t.Errorf("upgrade did not regenerate the shims: %v", err)
	}
}

func TestStableExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A stable symlink to the running binary, as Homebrew links bin/driftr to its Cellar copy.
	link := filepath.Join(t.TempDir(), "driftr")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "driftr")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := os.Args[0]
	t.Cleanup(func() { os.Args[0] = orig })
	for _, tt := range []struct{ invoked, want string }{
		{link, link}, // invoked through the symlink: keep it
		{other, exe}, // a different file: never trust it
		{"driftr-not-on-path-xyz", exe},
	} {
		os.Args[0] = tt.invoked
		if got, err := stableExecutable(); err != nil || got != tt.want {
			t.Errorf("invoked as %s: got %q, %v; want %q", tt.invoked, got, err, tt.want)
		}
	}
}

func TestGenerateShims_ReleasesTheOldHomebrewKeg(t *testing.T) {
	for _, tt := range []struct {
		name     string
		kegPath  []string // under a temp root
		released bool
	}{
		{"old Cellar keg", []string{"Cellar", "driftr", "1.2.1"}, true},
		{"not a Homebrew keg", []string{"opt", "driftr", "1.2.1"}, false},
		{"another formula's keg", []string{"Cellar", "node", "22.0.0"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			binDir, _ := ShimDir()
			keg := filepath.Join(append([]string{t.TempDir()}, tt.kegPath...)...)
			if err := os.MkdirAll(filepath.Join(keg, "bin"), 0o755); err != nil {
				t.Fatal(err)
			}
			keepme := filepath.Join(keg, ".keepme")
			if err := os.WriteFile(keepme, []byte(keg+"/bin/driftr\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			// Shims written by a driftr that baked the versioned keg path.
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := writeShim(binDir, "node", filepath.Join(keg, "bin", "driftr")); err != nil {
				t.Fatal(err)
			}

			if _, err := GenerateShims("test"); err != nil {
				t.Fatal(err)
			}

			_, err := os.Stat(keepme)
			if released := os.IsNotExist(err); released != tt.released {
				t.Errorf(".keepme released = %v, want %v", released, tt.released)
			}
		})
	}
}

// The driftr path is baked into every shim; a $, backtick, quote or backslash used to break them.
func TestWriteShim_PathWithShellMetacharacters(t *testing.T) {
	dir := t.TempDir()
	weird := filepath.Join(dir, "a $HOME `x` \"q\" \\b 'c'")
	if err := os.MkdirAll(weird, 0o755); err != nil {
		t.Fatal(err)
	}
	driftrBin := filepath.Join(weird, "driftr")
	if err := os.WriteFile(driftrBin, []byte("#!/bin/sh\necho OK \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeShim(dir, "node", driftrBin); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("/bin/sh", filepath.Join(dir, "node"), "-v").CombinedOutput()
	if err != nil || string(out) != "OK shim node -v\n" {
		t.Errorf("shim broken for %q: %q, %v", driftrBin, out, err)
	}
	if got := BakedPath(filepath.Join(dir, "node")); got != driftrBin {
		t.Errorf("BakedPath() = %q, want %q", got, driftrBin)
	}
	if !isShimFile(filepath.Join(dir, "node"), "node") {
		t.Error("isShimFile() = false for a shim writeShim made")
	}
}

// Shims from before single-quoting must still parse, so they refresh and release the Homebrew keg.
func TestParseShim(t *testing.T) {
	for _, tt := range []struct {
		content, path, tool string
		ok                  bool
	}{
		{"#!/bin/sh\nexec \"/old/driftr\" shim node \"$@\"\n", "/old/driftr", "node", true},
		{"#!/bin/sh\nexec '/new/driftr' shim tsc \"$@\"\n", "/new/driftr", "tsc", true},
		{"#!/bin/sh\nexec '/O'\\''Brien/driftr' shim npm \"$@\"\n", "/O'Brien/driftr", "npm", true},
		{"#!/bin/sh\nexec /bare/driftr shim node \"$@\"\n", "", "", false},
		{"#!/bin/sh\nexec '/x' shim node \"$@\"\necho extra\n", "", "", false},
		{"#!/bin/bash\nexec '/x' shim node \"$@\"\n", "", "", false},
	} {
		path, tool, ok := parseShim(tt.content)
		if path != tt.path || tool != tt.tool || ok != tt.ok {
			t.Errorf("parseShim(%q) = %q, %q, %v; want %q, %q, %v", tt.content, path, tool, ok, tt.path, tt.tool, tt.ok)
		}
	}
}
