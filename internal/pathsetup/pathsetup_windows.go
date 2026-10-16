package pathsetup

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/stackmade/driftr/internal/platform"
)

// userEnvKey is the key under HKEY_CURRENT_USER that Windows builds each new
// process's user environment from. Tests point it at a scratch key.
var userEnvKey = `Environment`

// targetName is how Result.Target names the user Path value.
const targetName = `HKCU\Environment\Path`

// announce tells running programs, Explorer first, that the environment
// changed, so what they start next sees the new Path. Tests replace it.
var announce = broadcastEnvironmentChange

// Detect reports whether the user Path in the registry has binDir. Shell rc
// files play no part on Windows: cmd, PowerShell and Git Bash all start from
// the environment Explorer hands them.
func Detect(binDir string) (Result, error) {
	path, _, err := readUserPath()
	if err != nil {
		return Result{}, err
	}
	return Result{
		Shell:         DetectShell(),
		BinDir:        binDir,
		Target:        targetName,
		InTarget:      pathListHas(path, binDir),
		InProcessPATH: pathContains(os.Getenv("PATH"), binDir),
	}, nil
}

// Apply puts binDir at the front of the user Path, keeping the value's
// registry type, and announces the change. Terminals that are already open
// keep their old PATH.
func Apply(r Result) (wrote bool, target string, err error) {
	if !r.NeedsFix() {
		return false, "", nil
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, userEnvKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, "", fmt.Errorf("open %s: %w", targetName, err)
	}
	defer k.Close()

	path, typ, err := getPath(k)
	if err != nil {
		return false, "", err
	}
	if pathListHas(path, r.BinDir) {
		return false, "", nil
	}
	newPath := r.BinDir
	if path != "" {
		newPath += ";" + path
	}
	if typ == registry.SZ {
		err = k.SetStringValue("Path", newPath)
	} else {
		err = k.SetExpandStringValue("Path", newPath)
	}
	if err != nil {
		return false, "", fmt.Errorf("write %s: %w", targetName, err)
	}
	announce()
	return true, targetName, nil
}

// ApplyGuard cannot help on Windows. A process's PATH lists the system
// entries before the user ones, so a Node.js installed for all users comes
// first, and only an administrator can reorder the system Path.
func ApplyGuard(Shell, string) (bool, string, error) {
	return false, "", errors.New("the system PATH comes before the user PATH on Windows: uninstall the other Node.js, or move its directory after driftr's in the system PATH, which needs an administrator")
}

// readUserPath returns the user Path value and its registry type. A missing
// key or value is an empty Path.
func readUserPath() (string, uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", targetName, err)
	}
	defer k.Close()
	return getPath(k)
}

func getPath(k registry.Key) (string, uint32, error) {
	path, typ, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("read %s: %w", targetName, err)
	}
	return path, typ, nil
}

// pathListHas reports whether the registry Path value list has dir as an
// entry, after expanding variables such as %USERPROFILE%.
func pathListHas(list, dir string) bool {
	for _, entry := range strings.Split(list, ";") {
		entry = strings.Trim(strings.TrimSpace(entry), `"`)
		if entry == "" {
			continue
		}
		if expanded, err := registry.ExpandString(entry); err == nil {
			entry = expanded
		}
		if platform.SamePath(entry, dir) {
			return true
		}
	}
	return false
}

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	// Best effort: a program that ignores the message picks the change up
	// when it next starts.
	_, _, _ = sendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, 0)
}
