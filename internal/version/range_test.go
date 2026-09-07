package version

import "testing"

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", s, err)
	}
	return v
}

func TestParseRange_Matches(t *testing.T) {
	tests := []struct {
		name    string
		rng     string
		matches []string
		misses  []string
	}{
		{
			name:    "bare major",
			rng:     "18",
			matches: []string{"18.0.0", "18.20.4"},
			misses:  []string{"17.9.9", "19.0.0"},
		},
		{
			name:    "bare major.minor",
			rng:     "18.1",
			matches: []string{"18.1.0", "18.1.9"},
			misses:  []string{"18.0.9", "18.2.0"},
		},
		{
			name:    "bare exact",
			rng:     "18.1.0",
			matches: []string{"18.1.0"},
			misses:  []string{"18.1.1", "18.0.0"},
		},
		{
			name:    "equals",
			rng:     "=20.11.1",
			matches: []string{"20.11.1"},
			misses:  []string{"20.11.0", "20.12.0"},
		},
		{
			name:    "greater or equal",
			rng:     ">=18",
			matches: []string{"18.0.0", "22.14.0"},
			misses:  []string{"17.9.9"},
		},
		{
			name:    "greater or equal, detached operator",
			rng:     ">= 18.12.0",
			matches: []string{"18.12.0", "20.0.0"},
			misses:  []string{"18.11.9"},
		},
		{
			name:    "greater than partial",
			rng:     ">18",
			matches: []string{"19.0.0"},
			misses:  []string{"18.20.4", "18.0.0"},
		},
		{
			name:    "greater than exact",
			rng:     ">18.1.0",
			matches: []string{"18.1.1", "20.0.0"},
			misses:  []string{"18.1.0"},
		},
		{
			name:    "less than",
			rng:     "<21",
			matches: []string{"20.11.1", "1.0.0"},
			misses:  []string{"21.0.0"},
		},
		{
			name:    "less or equal partial",
			rng:     "<=20",
			matches: []string{"20.11.1"},
			misses:  []string{"21.0.0"},
		},
		{
			name:    "less or equal exact",
			rng:     "<=20.11.1",
			matches: []string{"20.11.1"},
			misses:  []string{"20.11.2"},
		},
		{
			name:    "caret",
			rng:     "^20.9.0",
			matches: []string{"20.9.0", "20.18.2"},
			misses:  []string{"20.8.9", "21.0.0"},
		},
		{
			name:    "caret partial",
			rng:     "^20",
			matches: []string{"20.0.0", "20.18.2"},
			misses:  []string{"21.0.0"},
		},
		{
			name:    "caret zero major",
			rng:     "^0.2.3",
			matches: []string{"0.2.3", "0.2.9"},
			misses:  []string{"0.3.0", "0.2.2"},
		},
		{
			name:    "tilde",
			rng:     "~20.9.0",
			matches: []string{"20.9.0", "20.9.7"},
			misses:  []string{"20.10.0", "20.8.9"},
		},
		{
			name:    "tilde major only",
			rng:     "~20",
			matches: []string{"20.0.0", "20.11.1"},
			misses:  []string{"21.0.0"},
		},
		{
			name:    "wildcard x",
			rng:     "18.x",
			matches: []string{"18.0.0", "18.20.4"},
			misses:  []string{"19.0.0"},
		},
		{
			name:    "wildcard star on minor",
			rng:     "18.*",
			matches: []string{"18.4.0"},
			misses:  []string{"17.0.0"},
		},
		{
			name:    "wildcard capital X on patch",
			rng:     "18.1.X",
			matches: []string{"18.1.4"},
			misses:  []string{"18.2.0"},
		},
		{
			name:    "bare star matches everything",
			rng:     "*",
			matches: []string{"0.0.1", "24.3.0"},
		},
		{
			name:    "and",
			rng:     ">=16 <21",
			matches: []string{"16.0.0", "20.11.1"},
			misses:  []string{"15.9.9", "21.0.0"},
		},
		{
			name:    "or",
			rng:     "18 || 20",
			matches: []string{"18.20.4", "20.0.0"},
			misses:  []string{"19.0.0", "21.0.0"},
		},
		{
			name:    "or of and groups",
			rng:     ">=16 <17 || >=20 <21",
			matches: []string{"16.5.0", "20.11.1"},
			misses:  []string{"18.0.0", "21.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRange(tt.rng)
			if err != nil {
				t.Fatalf("ParseRange(%q) error: %v", tt.rng, err)
			}
			for _, m := range tt.matches {
				if !r.Matches(mustParse(t, m)) {
					t.Errorf("%q should match %q", tt.rng, m)
				}
			}
			for _, m := range tt.misses {
				if r.Matches(mustParse(t, m)) {
					t.Errorf("%q should not match %q", tt.rng, m)
				}
			}
		})
	}
}

func TestParseRange_Rejected(t *testing.T) {
	tests := []struct {
		name string
		rng  string
	}{
		{"hyphen range", "18 - 20"},
		{"pre-release tag", ">=18.0.0-rc.1"},
		{"build metadata", "18.0.0+build.1"},
		{"empty", ""},
		{"whitespace only", "   "},
		{"garbage", "not-a-version"},
		{"nonsense word", "latest"},
		{"operator without version", ">="},
		{"trailing or", "18 ||"},
		{"bad component", "18.abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseRange(tt.rng); err == nil {
				t.Errorf("ParseRange(%q) = nil error, want error", tt.rng)
			}
		})
	}
}

func TestRange_LowerBoundMajor(t *testing.T) {
	tests := []struct {
		rng  string
		want int
		ok   bool
	}{
		{">=18", 18, true},
		{"^20.9.0", 20, true},
		{"18 || 20", 18, true},
		{"20 || 18", 18, true},
		{">=16 <21", 16, true},
		{">18", 19, true},
		{"*", 0, false},
		{"<21", 0, false},
		{"18 || <21", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.rng, func(t *testing.T) {
			r, err := ParseRange(tt.rng)
			if err != nil {
				t.Fatalf("ParseRange(%q) error: %v", tt.rng, err)
			}
			got, ok := r.LowerBoundMajor()
			if ok != tt.ok || (ok && got != tt.want) {
				t.Errorf("LowerBoundMajor() = (%d, %v), want (%d, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
