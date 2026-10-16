package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestColorFlags_MatchesFlagColumn(t *testing.T) {
	usages := "      --bump      move pins\n  -h, --help      help for pin\n      --dry-run   with --bump, show\n"
	matches := flagName.FindAllStringSubmatch(usages, -1)
	got := make([]string, 0, len(matches))
	for _, m := range matches {
		got = append(got, m[2])
	}
	want := []string{"--bump", "-h, --help", "--dry-run"}
	if len(got) != len(want) {
		t.Fatalf("matched %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Without color, help must match cobra's default so piped help and the e2e suite see no change.
func TestUsageTemplate_PlainMatchesCobraDefault(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	usage := func(colored bool, args ...string) string {
		root := NewRootCmd()
		if !colored {
			root.SetUsageTemplate((&cobra.Command{}).UsageTemplate())
		}
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetArgs(append(args, "--help"))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	for _, args := range [][]string{{}, {"install"}, {"node"}, {"pin"}} {
		if got, want := usage(true, args...), usage(false, args...); got != want {
			t.Errorf("driftr %v --help differs from cobra's default:\n got: %q\nwant: %q", args, got, want)
		}
	}
}
