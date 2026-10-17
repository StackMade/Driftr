package pathsetup

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// scratchEnv points the user environment key at a throwaway key, holding Path
// if path is not "", and returns how many times Apply announced a change.
func scratchEnv(t *testing.T, path string, typ uint32) *int {
	t.Helper()
	key := fmt.Sprintf(`Software\driftr-test-%d`, time.Now().UnixNano())
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		if typ == registry.SZ {
			err = k.SetStringValue("Path", path)
		} else {
			err = k.SetExpandStringValue("Path", path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	k.Close()
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, key) })

	oldKey, oldAnnounce := userEnvKey, announce
	announced := 0
	userEnvKey, announce = key, func() { announced++ }
	t.Cleanup(func() { userEnvKey, announce = oldKey, oldAnnounce })
	return &announced
}

func userPath(t *testing.T) (string, uint32) {
	t.Helper()
	path, typ, err := readUserPath()
	if err != nil {
		t.Fatal(err)
	}
	return path, typ
}

func TestApply_PrependsToUserPath(t *testing.T) {
	announced := scratchEnv(t, `C:\Tools;%USERPROFILE%\bin`, registry.EXPAND_SZ)
	binDir := `C:\Users\me\.driftr\bin`

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.NeedsFix() || r.Target != targetName {
		t.Fatalf("Detect() = %+v, want a fix in %s", r, targetName)
	}
	wrote, target, err := Apply(r)
	if err != nil || !wrote || target != targetName {
		t.Fatalf("Apply() = %v, %q, %v", wrote, target, err)
	}
	path, typ := userPath(t)
	if path != binDir+`;C:\Tools;%USERPROFILE%\bin` || typ != registry.EXPAND_SZ {
		t.Errorf("Path = %q (type %d), want binDir first and the type kept", path, typ)
	}
	if *announced != 1 {
		t.Errorf("announced %d times, want once", *announced)
	}

	// Configured now: nothing more to do.
	if r, _ := Detect(binDir); r.NeedsFix() {
		t.Error("Detect() still wants a fix after Apply")
	}
}

func TestDetect_ExpandsAndIgnoresCase(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	scratchEnv(t, `C:\Tools;"%USERPROFILE%\.DRIFTR\bin"`, registry.EXPAND_SZ)

	r, err := Detect(home + `\.driftr\bin`)
	if err != nil {
		t.Fatal(err)
	}
	if r.NeedsFix() {
		t.Error("an entry naming USERPROFILE, in other case, was not recognized")
	}
}

func TestApply_NoPathValueYet(t *testing.T) {
	scratchEnv(t, "", 0)
	binDir := `C:\Users\me\.driftr\bin`

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Apply(r); err != nil {
		t.Fatal(err)
	}
	if path, typ := userPath(t); path != binDir || typ != registry.EXPAND_SZ {
		t.Errorf("Path = %q (type %d), want only binDir as REG_EXPAND_SZ", path, typ)
	}
}

func TestApplyGuard_ExplainsSystemPath(t *testing.T) {
	wrote, _, err := ApplyGuard(DetectShell(), `C:\x`)
	if wrote || err == nil || !strings.Contains(err.Error(), "system PATH") {
		t.Errorf("ApplyGuard() = %v, %v; want an explanation", wrote, err)
	}
}
