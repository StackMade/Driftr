package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major  int
	Minor  int
	Patch  int
	Raw    string
	Latest bool // true when input was "latest" or "node@latest"
	LTS    bool // true when input was "lts", "lts/*" or "lts/<codename>"

	// LTSCodename holds the codename from an "lts/<codename>" alias,
	// lowercased. Empty for plain "lts" and "lts/*".
	LTSCodename string
}

// ltsCodenameMajor maps Node.js LTS codenames to their major version.
// The network install path resolves codenames from the release index, so this
// table only serves offline resolution (the shim hot path must never hit the
// network) and a missing entry degrades gracefully: the alias is simply not
// resolvable offline.
var ltsCodenameMajor = map[string]int{
	"argon":    4,
	"boron":    6,
	"carbon":   8,
	"dubnium":  10,
	"erbium":   12,
	"fermium":  14,
	"gallium":  16,
	"hydrogen": 18,
	"iron":     20,
	"jod":      22,
	"krypton":  24,
}

// LTSCodenameMajor returns the major version for a Node.js LTS codename.
// The lookup is case-insensitive; ok is false for unknown codenames.
func LTSCodenameMajor(name string) (int, bool) {
	major, ok := ltsCodenameMajor[strings.ToLower(name)]
	return major, ok
}

// stripToolPrefix removes an optional "tool@" prefix (e.g. "node@24" → "24").
func stripToolPrefix(s string) string {
	if _, after, ok := strings.Cut(s, "@"); ok {
		return after
	}
	return s
}

// Parse parses a version string like "24", "24.0", or "24.0.1".
// Supports optional "v" prefix and "tool@" prefix (e.g. "node@24").
func Parse(s string) (Version, error) {
	raw := s
	s = stripToolPrefix(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimSpace(s)

	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	if s == "latest" {
		return Version{Raw: raw, Latest: true}, nil
	}

	// LTS aliases: "lts", "lts/*" (newest LTS) and "lts/<codename>".
	if lower := strings.ToLower(s); lower == "lts" || lower == "lts/*" {
		return Version{Raw: raw, LTS: true}, nil
	} else if name, ok := strings.CutPrefix(lower, "lts/"); ok && name != "" {
		return Version{Raw: raw, LTS: true, LTSCodename: name}, nil
	}

	parts := strings.SplitN(s, ".", 3)

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}

	v := Version{Major: major, Raw: raw}

	if len(parts) >= 2 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return Version{}, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
		}
		v.Minor = minor
	}

	if len(parts) == 3 {
		patch, err := strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version %q: %w", parts[2], err)
		}
		v.Patch = patch
	}

	return v, nil
}

// IsPartial returns true if the version was specified without all three components.
func (v Version) IsPartial() bool {
	raw := stripToolPrefix(v.Raw)
	raw = strings.TrimPrefix(raw, "v")
	parts := strings.Split(raw, ".")
	return len(parts) < 3
}

// String returns the full semver string.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// MajorMinor returns "MAJOR.MINOR" format.
func (v Version) MajorMinor() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Matches returns true if v (potentially partial) matches other (full version).
// "24" matches any 24.x.x, "24.14" matches any 24.14.x, "24.14.1" is exact.
// A Latest version matches everything.
func (v Version) Matches(other Version) bool {
	if v.Latest {
		return true
	}
	if v.Major != other.Major {
		return false
	}
	raw := stripToolPrefix(v.Raw)
	raw = strings.TrimPrefix(raw, "v")
	parts := strings.Split(raw, ".")
	if len(parts) == 1 {
		return true // major-only match
	}
	if v.Minor != other.Minor {
		return false
	}
	if len(parts) == 2 {
		return true // major.minor match
	}
	return v.Patch == other.Patch
}

// MatchesMajor returns true if the other version has the same major version.
func (v Version) MatchesMajor(other Version) bool {
	return v.Major == other.Major
}
