package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Range is a small subset of the semver range syntax, enough for the
// "engines.node" field of a package.json.
//
// Supported: the comparators =, >, >=, < and <=, the caret (^) and tilde (~)
// operators, the wildcards x, X and *, a bare version ("18", "18.1", "18.1.0"),
// whitespace as AND and || as OR.
//
// Not supported: hyphen ranges ("18 - 20") and pre-release tags. Both are
// rejected by ParseRange rather than silently mis-parsed.
type Range struct {
	Raw string
	ors [][]bound // OR of AND-groups
}

// bound is one comparator, normalized to a half-open interval [min, max).
// A nil min or max means that side is unbounded.
type bound struct {
	min *Version
	max *Version
}

// ParseRange parses a semver range from the supported subset.
func ParseRange(s string) (Range, error) {
	raw := s
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{}, fmt.Errorf("empty range")
	}
	if strings.Contains(s, "-") {
		return Range{}, fmt.Errorf("unsupported syntax in %q: hyphen ranges and pre-release tags", raw)
	}
	if strings.Contains(s, "+") {
		return Range{}, fmt.Errorf("unsupported syntax in %q: build metadata", raw)
	}

	r := Range{Raw: raw}
	for _, group := range strings.Split(s, "||") {
		fields, err := splitComparators(group)
		if err != nil {
			return Range{}, fmt.Errorf("invalid range %q: %w", raw, err)
		}
		var bounds []bound
		for _, f := range fields {
			b, err := parseComparator(f)
			if err != nil {
				return Range{}, fmt.Errorf("invalid range %q: %w", raw, err)
			}
			bounds = append(bounds, b)
		}
		r.ors = append(r.ors, bounds)
	}
	return r, nil
}

// splitComparators splits an AND-group on whitespace, re-joining an operator
// that was written detached from its version (">= 18").
func splitComparators(group string) ([]string, error) {
	fields := strings.Fields(group)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty comparator set")
	}
	var out []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if isOperator(f) {
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("operator %q without a version", f)
			}
			i++
			f += fields[i]
		}
		out = append(out, f)
	}
	return out, nil
}

func isOperator(s string) bool {
	switch s {
	case "=", ">", ">=", "<", "<=", "^", "~":
		return true
	}
	return false
}

func parseComparator(s string) (bound, error) {
	op := ""
	for _, prefix := range []string{">=", "<=", "^", "~", ">", "<", "="} {
		if strings.HasPrefix(s, prefix) {
			op = prefix
			s = strings.TrimPrefix(s, prefix)
			break
		}
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")

	lo, hi, any, err := parsePartial(s)
	if err != nil {
		return bound{}, err
	}

	// A wildcard version ignores its operator: ">=*" is just "everything".
	if any {
		return bound{}, nil
	}

	switch op {
	case "", "=":
		if hi == nil { // fully specified: exactly this version
			next := lo
			next.Patch++
			hi = &next
		}
		return bound{min: &lo, max: hi}, nil
	case ">=":
		return bound{min: &lo}, nil
	case ">":
		if hi == nil { // fully specified: strictly greater than lo
			next := lo
			next.Patch++
			return bound{min: &next}, nil
		}
		return bound{min: hi}, nil // partial: greater than all of 18.x
	case "<":
		return bound{max: &lo}, nil
	case "<=":
		if hi == nil {
			next := lo
			next.Patch++
			return bound{max: &next}, nil
		}
		return bound{max: hi}, nil
	case "^":
		return bound{min: &lo, max: caretMax(lo, s)}, nil
	case "~":
		if strings.Count(s, ".") == 0 { // "~18" behaves like "^18"
			return bound{min: &lo, max: hi}, nil
		}
		return bound{min: &lo, max: &Version{Major: lo.Major, Minor: lo.Minor + 1}}, nil
	}
	return bound{}, fmt.Errorf("unsupported operator %q", op)
}

// caretMax implements npm's caret rule: the leftmost non-zero component is
// what stays pinned.
func caretMax(lo Version, raw string) *Version {
	parts := strings.Count(raw, ".") + 1
	switch {
	case lo.Major != 0:
		return &Version{Major: lo.Major + 1}
	case parts == 1: // "^0" → <1.0.0
		return &Version{Major: 1}
	case lo.Minor != 0 || parts == 2: // "^0.2.3" and "^0.0" → <0.(minor+1).0
		return &Version{Major: 0, Minor: lo.Minor + 1}
	default: // "^0.0.3" → <0.0.4
		return &Version{Patch: lo.Patch + 1}
	}
}

// parsePartial turns "18", "18.1", "18.1.0", "18.x" or "*" into the version it
// starts at and, when it is not fully specified, the first version past it.
// any is true for a bare wildcard, which matches everything.
func parsePartial(s string) (lo Version, hi *Version, any bool, err error) {
	if s == "" {
		return Version{}, nil, false, fmt.Errorf("missing version")
	}
	if isWildcard(s) {
		return Version{}, nil, true, nil
	}

	parts := strings.SplitN(s, ".", 3)
	nums := make([]int, 0, 3)
	for _, p := range parts {
		if isWildcard(p) {
			break
		}
		n, convErr := strconv.Atoi(p)
		if convErr != nil {
			return Version{}, nil, false, fmt.Errorf("invalid version component %q", p)
		}
		if n < 0 {
			return Version{}, nil, false, fmt.Errorf("invalid version component %q", p)
		}
		nums = append(nums, n)
	}

	switch len(nums) {
	case 0:
		return Version{}, nil, false, fmt.Errorf("invalid version %q", s)
	case 1:
		lo = Version{Major: nums[0]}
		return lo, &Version{Major: nums[0] + 1}, false, nil
	case 2:
		lo = Version{Major: nums[0], Minor: nums[1]}
		return lo, &Version{Major: nums[0], Minor: nums[1] + 1}, false, nil
	default:
		return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil, false, nil
	}
}

func isWildcard(s string) bool {
	return s == "x" || s == "X" || s == "*"
}

// Matches reports whether v satisfies the range.
func (r Range) Matches(v Version) bool {
	for _, group := range r.ors {
		ok := true
		for _, b := range group {
			if b.min != nil && compare(v, *b.min) < 0 {
				ok = false
				break
			}
			if b.max != nil && compare(v, *b.max) >= 0 {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// LowerBoundMajor returns the major version the range starts at, if it has a
// lower bound at all. Used to suggest something concrete to install.
func (r Range) LowerBoundMajor() (int, bool) {
	best, found := 0, false
	for _, group := range r.ors {
		var groupMin *Version
		for _, b := range group {
			if b.min == nil {
				continue
			}
			if groupMin == nil || compare(*b.min, *groupMin) > 0 {
				groupMin = b.min
			}
		}
		if groupMin == nil {
			return 0, false // one alternative is open-ended downwards
		}
		if !found || groupMin.Major < best {
			best, found = groupMin.Major, true
		}
	}
	return best, found
}

func compare(a, b Version) int {
	if a.Major != b.Major {
		return a.Major - b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor - b.Minor
	}
	return a.Patch - b.Patch
}
