package cli

import "testing"

func TestParseToolVersion(t *testing.T) {
	tests := []struct {
		input    string
		wantTool string
		wantVer  string
	}{
		{"node@24.0.0", "node", "24.0.0"},
		{"node@24", "node", "24"},
		{"pnpm@9.15.0", "pnpm", "9.15.0"},
		{"yarn@1.22.22", "yarn", "1.22.22"},
		{"node@latest", "node", "latest"},
		{"pnpm@latest", "pnpm", "latest"},
		{"24.0.0", "node", "24.0.0"},
		{"24", "node", "24"},
		{"latest", "node", "latest"},
		{"node@v24.0.0", "node", "v24.0.0"},
		// A bare tool name means that tool at latest, not a node version named after the tool.
		{"pnpm", "pnpm", ""},
		{"yarn", "yarn", ""},
		{"node", "node", ""},
		{"npm", "npm", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tool, ver := parseToolVersion(tt.input)
			if tool != tt.wantTool {
				t.Errorf("parseToolVersion(%q) tool = %q, want %q", tt.input, tool, tt.wantTool)
			}
			if ver != tt.wantVer {
				t.Errorf("parseToolVersion(%q) ver = %q, want %q", tt.input, ver, tt.wantVer)
			}
		})
	}
}
