package cli

import (
	"regexp"
	"testing"

	"github.com/stackmade/driftr/v2/internal/agents"
)

// The guide is prose the compiler cannot check, so a renamed command would
// leave agents running one that no longer exists.
func TestGuideNamesRealCommands(t *testing.T) {
	root := NewRootCmd()
	matches := regexp.MustCompile("`driftr ([a-z][a-z-]*)").FindAllStringSubmatch(agents.Guide, -1)
	if len(matches) == 0 {
		t.Fatal("the guide names no commands")
	}
	for _, m := range matches {
		cmd, _, err := root.Find([]string{m[1]})
		if err != nil || cmd == root {
			t.Errorf("the guide names %q, which is not a command", "driftr "+m[1])
		}
	}
}
