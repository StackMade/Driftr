package cli

import "testing"

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
