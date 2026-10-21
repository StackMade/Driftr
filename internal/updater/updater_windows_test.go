package updater

import (
	"os"
	"path/filepath"
	"testing"
)

// A shim started before an update keeps running the copy that update moved
// aside, and Windows will not delete it; the next update must still work.
func TestReplaceBinary_SecondUpdateWhileOldCopyRuns(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "driftr.exe")
	if err := os.WriteFile(oldPath, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i, body := range []string{"v2", "v3"} {
		newPath := filepath.Join(dir, "new")
		if err := os.WriteFile(newPath, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := replaceBinary(newPath, oldPath); err != nil {
			t.Fatalf("update %d: %v", i+1, err)
		}
		// Hold every copy open, as a running image would be.
		old, _ := filepath.Glob(oldPath + ".*.old")
		for _, f := range old {
			h, err := os.Open(f)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
		}
	}
	if got, _ := os.ReadFile(oldPath); string(got) != "v3" {
		t.Errorf("binary = %q, want v3", got)
	}
}
