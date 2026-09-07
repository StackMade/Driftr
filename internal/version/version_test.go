package version

import (
	"strings"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	tests := []struct {
		input   string
		major   int
		minor   int
		patch   int
		str     string
		partial bool
	}{
		{"24", 24, 0, 0, "24.0.0", true},
		{"24.0", 24, 0, 0, "24.0.0", true},
		{"24.0.1", 24, 0, 1, "24.0.1", false},
		{"v24.0.1", 24, 0, 1, "24.0.1", false},
		{"node@24", 24, 0, 0, "24.0.0", true},
		{"node@v24.0.1", 24, 0, 1, "24.0.1", false},
		{"0.12.18", 0, 12, 18, "0.12.18", false},
		{"v22", 22, 0, 0, "22.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if v.Major != tt.major || v.Minor != tt.minor || v.Patch != tt.patch {
				t.Errorf("Parse(%q) = {%d, %d, %d}, want {%d, %d, %d}",
					tt.input, v.Major, v.Minor, v.Patch, tt.major, tt.minor, tt.patch)
			}
			if got := v.String(); got != tt.str {
				t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.str)
			}
			if got := v.IsPartial(); got != tt.partial {
				t.Errorf("Parse(%q).IsPartial() = %v, want %v", tt.input, got, tt.partial)
			}
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	tests := []string{
		"",
		"abc",
		"24.abc",
		"24.0.abc",
		"node@",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := Parse(input)
			if err == nil {
				t.Errorf("Parse(%q) expected error, got nil", input)
			}
		})
	}
}

func TestMajorMinor(t *testing.T) {
	v, _ := Parse("24.1.3")
	if got := v.MajorMinor(); got != "24.1" {
		t.Errorf("MajorMinor() = %q, want %q", got, "24.1")
	}
}

func TestParse_Latest(t *testing.T) {
	tests := []string{"latest", "node@latest"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			v, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", input, err)
			}
			if !v.Latest {
				t.Errorf("Parse(%q).Latest = false, want true", input)
			}
		})
	}
}

func TestParse_LTS(t *testing.T) {
	tests := []string{"lts", "node@lts"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			v, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", input, err)
			}
			if !v.LTS {
				t.Errorf("Parse(%q).LTS = false, want true", input)
			}
			if v.Latest {
				t.Errorf("Parse(%q).Latest = true, want false", input)
			}
		})
	}
}

func TestParse_LTSAliases(t *testing.T) {
	tests := []struct {
		input    string
		codename string
	}{
		{"lts", ""},
		{"node@lts", ""},
		{"lts/*", ""},
		{"node@lts/*", ""},
		{"lts/jod", "jod"},
		{"lts/JOD", "jod"},
		{"LTS/Iron", "iron"},
		{"lts/unknownname", "unknownname"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if !v.LTS {
				t.Errorf("Parse(%q).LTS = false, want true", tt.input)
			}
			if v.LTSCodename != tt.codename {
				t.Errorf("Parse(%q).LTSCodename = %q, want %q", tt.input, v.LTSCodename, tt.codename)
			}
		})
	}
}

func TestLTSCodenameMajor(t *testing.T) {
	tests := []struct {
		name  string
		major int
		ok    bool
	}{
		{"jod", 22, true},
		{"Iron", 20, true},
		{"argon", 4, true},
		{"krypton", 24, true},
		{"unknownname", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, ok := LTSCodenameMajor(tt.name)
			if major != tt.major || ok != tt.ok {
				t.Errorf("LTSCodenameMajor(%q) = (%d, %v), want (%d, %v)", tt.name, major, ok, tt.major, tt.ok)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
		want    bool
	}{
		{"24", "24.14.1", true},
		{"24", "24.0.0", true},
		{"24", "22.14.0", false},
		{"24.14", "24.14.1", true},
		{"24.14", "24.14.0", true},
		{"24.14", "24.13.0", false},
		{"24.14", "22.14.0", false},
		{"24.14.1", "24.14.1", true},
		{"24.14.1", "24.14.0", false},
		{"latest", "24.14.1", true},
		{"latest", "22.0.0", true},
		{"node@24", "24.14.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.target, func(t *testing.T) {
			p, err := Parse(tt.pattern)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.pattern, err)
			}
			tgt, err := Parse(tt.target)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.target, err)
			}
			if got := p.Matches(tgt); got != tt.want {
				t.Errorf("Parse(%q).Matches(Parse(%q)) = %v, want %v", tt.pattern, tt.target, got, tt.want)
			}
		})
	}
}

func TestMatchesMajor(t *testing.T) {
	a, _ := Parse("24.0.0")
	b, _ := Parse("24.1.3")
	c, _ := Parse("22.0.0")

	if !a.MatchesMajor(b) {
		t.Error("expected 24.0.0 to match major with 24.1.3")
	}
	if a.MatchesMajor(c) {
		t.Error("expected 24.0.0 to NOT match major with 22.0.0")
	}
}

// FuzzParse asserts that Parse never panics and that a fully specified,
// non-alias version survives a String()/Parse() round trip.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"24", "24.0", "24.0.1", "v24.0.1", "node@24", "node@v24.0.1",
		"0.12.18", "latest", "node@latest", "lts", "lts/*", "lts/jod", "LTS/Iron",
		"", " ", "abc", "24.abc", "node@", "v", "@", "@@@",
		"99999999999999999999.0.0", "-1.-1.-1", "+1.0.0", "1.2.3.4",
		"lts/", "lts//", "lts/lts/lts/lts", "lts/\x00",
		"24.0.1\x00", "２４.０.１", "١٢.٣.٤", "24.0.1\n", "  24.0.1  ",
		strings.Repeat("9", 400), strings.Repeat("1.", 200) + "1",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		v, err := Parse(s)
		if err != nil {
			return
		}
		// Every accessor must be safe on anything Parse accepted.
		_ = v.String()
		_ = v.MajorMinor()
		_ = v.Matches(v)
		_ = v.MatchesMajor(v)

		if v.Latest || v.LTS || v.IsPartial() {
			return
		}

		rt, err := Parse(v.String())
		if err != nil {
			t.Fatalf("Parse(%q) = %+v, but Parse(%q) failed: %v", s, v, v.String(), err)
		}
		if rt.Major != v.Major || rt.Minor != v.Minor || rt.Patch != v.Patch {
			t.Errorf("round trip of %q: Parse(%q) = {%d, %d, %d}, want {%d, %d, %d}",
				s, v.String(), rt.Major, rt.Minor, rt.Patch, v.Major, v.Minor, v.Patch)
		}
	})
}
