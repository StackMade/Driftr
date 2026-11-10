package cli

import "testing"

func TestCheckBatchArgs(t *testing.T) {
	tests := []struct {
		binary string
		args   []string
		ok     bool
	}{
		{`C:\pnpm\tsc.cmd`, []string{"--project", "a b", "x=1"}, true},
		{`C:\pnpm\tsc.exe`, []string{"a&calc", `"q"`}, true},
		{`C:\pnpm\tsc.cmd`, []string{"a&calc"}, false},
		{`C:\pnpm\TSC.CMD`, []string{"a|calc"}, false},
		{`C:\pnpm\tsc.bat`, []string{`x" & calc & "`}, false},
		{`C:\pnpm\tsc.cmd`, []string{"%PATH%"}, false},
		{`C:\pnpm\tsc.cmd`, []string{"a\nb"}, false},
		{`C:\pnpm\tsc.cmd`, nil, true},
	}
	for _, tt := range tests {
		if err := checkBatchArgs(tt.binary, tt.args); (err == nil) != tt.ok {
			t.Errorf("checkBatchArgs(%q, %q) = %v, want ok=%v", tt.binary, tt.args, err, tt.ok)
		}
	}
}

func TestIsGlobalNpm(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"install", "-g", "typescript"}, true},
		{[]string{"i", "--global", "typescript"}, true},
		{[]string{"uninstall", "--location=global", "typescript"}, true},
		{[]string{"install", "--location", "global", "typescript"}, true},
		{[]string{"install", "typescript"}, false},
		{[]string{"install", "--location", "project"}, false},
		// After "--" the flag belongs to the command npm runs, not to npm.
		{[]string{"exec", "--", "tool", "-g"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := isGlobalNpm(tt.args); got != tt.want {
			t.Errorf("isGlobalNpm(%q) = %v, want %v", tt.args, got, tt.want)
		}
	}
}
