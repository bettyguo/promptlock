// Package version implements the strict semver subset promptlock uses for prompt
// versions: MAJOR.MINOR.PATCH with optional `-PRERELEASE` and `+BUILD` metadata,
// per https://semver.org/. We don't need a 3rd-party dep; the parsing rules are
// simple and we want the binary small.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semver value. Pre-release and BuildMetadata are stored as
// raw strings (no further parsing) because we never compare on them per spec —
// build metadata is ignored for ordering, and pre-release ordering is rarely
// needed for prompt versions.
type Version struct {
	Major, Minor, Patch uint64
	PreRelease          string // e.g. "rc.1", without the leading '-'
	BuildMetadata       string // e.g. "exp.sha.5114f85", without the leading '+'
}

// Parse parses a semver string. Returns a Version on success, error on failure.
func Parse(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("version: empty string")
	}
	v := Version{}

	// Split off build metadata first ('+' may not appear in pre-release per spec).
	if i := strings.Index(s, "+"); i >= 0 {
		v.BuildMetadata = s[i+1:]
		if v.BuildMetadata == "" {
			return Version{}, fmt.Errorf("version: empty build metadata in %q", s)
		}
		if !isValidIdentifierSeq(v.BuildMetadata, true) {
			return Version{}, fmt.Errorf("version: invalid build metadata %q", v.BuildMetadata)
		}
		s = s[:i]
	}
	// Then split off pre-release.
	if i := strings.Index(s, "-"); i >= 0 {
		v.PreRelease = s[i+1:]
		if v.PreRelease == "" {
			return Version{}, fmt.Errorf("version: empty pre-release in %q", s)
		}
		if !isValidIdentifierSeq(v.PreRelease, false) {
			return Version{}, fmt.Errorf("version: invalid pre-release %q", v.PreRelease)
		}
		s = s[:i]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version: expected MAJOR.MINOR.PATCH, got %q", s)
	}
	var err error
	if v.Major, err = parseNumeric(parts[0]); err != nil {
		return Version{}, fmt.Errorf("version: major: %w", err)
	}
	if v.Minor, err = parseNumeric(parts[1]); err != nil {
		return Version{}, fmt.Errorf("version: minor: %w", err)
	}
	if v.Patch, err = parseNumeric(parts[2]); err != nil {
		return Version{}, fmt.Errorf("version: patch: %w", err)
	}
	return v, nil
}

// String renders the canonical semver form.
func (v Version) String() string {
	out := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		out += "-" + v.PreRelease
	}
	if v.BuildMetadata != "" {
		out += "+" + v.BuildMetadata
	}
	return out
}

// BumpMajor returns v with major+1 and minor/patch reset, dropping pre-release/build.
func (v Version) BumpMajor() Version { return Version{Major: v.Major + 1} }

// BumpMinor returns v with minor+1 and patch reset, dropping pre-release/build.
func (v Version) BumpMinor() Version { return Version{Major: v.Major, Minor: v.Minor + 1} }

// BumpPatch returns v with patch+1, dropping pre-release/build.
func (v Version) BumpPatch() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
}

// Compare returns -1 / 0 / +1 per semver precedence, ignoring build metadata.
// Pre-release versions sort before the corresponding release (1.0.0-rc < 1.0.0).
func (v Version) Compare(o Version) int {
	if c := cmpUint(v.Major, o.Major); c != 0 {
		return c
	}
	if c := cmpUint(v.Minor, o.Minor); c != 0 {
		return c
	}
	if c := cmpUint(v.Patch, o.Patch); c != 0 {
		return c
	}
	// Pre-release rules: a version with pre-release < without; otherwise field-by-field.
	switch {
	case v.PreRelease == "" && o.PreRelease == "":
		return 0
	case v.PreRelease == "":
		return 1
	case o.PreRelease == "":
		return -1
	}
	return strings.Compare(v.PreRelease, o.PreRelease)
}

func parseNumeric(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty numeric component")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("numeric component %q has leading zero", s)
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a non-negative integer: %q", s)
	}
	return n, nil
}

// isValidIdentifierSeq validates a dot-separated identifier sequence per the
// semver spec. allowAllNumeric controls whether all-numeric identifiers may have
// leading zeros (true for build metadata, false for pre-release).
func isValidIdentifierSeq(s string, allowLeadingZeroNumeric bool) bool {
	for _, part := range strings.Split(s, ".") {
		if part == "" {
			return false
		}
		allNumeric := true
		for _, r := range part {
			switch {
			case r >= '0' && r <= '9':
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
				allNumeric = false
			default:
				return false
			}
		}
		if allNumeric && !allowLeadingZeroNumeric && len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	return true
}

func cmpUint(a, b uint64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
