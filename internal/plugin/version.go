package plugin

import (
	"fmt"
	"strconv"
	"strings"
)

// CoreVersion 현재 Angple Core 버전
const CoreVersion = "1.0.0"

// SemVer 시맨틱 버전
type SemVer struct {
	Major int
	Minor int
	Patch int
}

// ParseSemVer 버전 문자열 파싱 (프리릴리즈 태그 무시)
func ParseSemVer(s string) (SemVer, error) {
	s = strings.TrimSpace(s)
	// 프리릴리즈 태그 제거 (e.g., "1.0.0-beta" → "1.0.0")
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return SemVer{}, fmt.Errorf("invalid semver: %q (expected x.y.z)", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid major version: %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid minor version: %q", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid patch version: %q", parts[2])
	}

	return SemVer{Major: major, Minor: minor, Patch: patch}, nil
}

// Compare 비교: -1 (v < other), 0 (v == other), 1 (v > other)
func (v SemVer) Compare(other SemVer) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

func (v SemVer) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// VersionConstraint 버전 제약 조건
type VersionConstraint struct {
	Op      string // ">=", "<", "~", "^"
	Version SemVer
}

// ParseVersionRange 버전 범위 문자열 파싱
// 지원 형식: ">=1.0.0", ">=1.0.0 <2.0.0", "~1.2.0", "^1.2.0"
func ParseVersionRange(s string) ([]VersionConstraint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty version range")
	}

	tokens := strings.Fields(s)
	constraints := make([]VersionConstraint, 0, len(tokens))

	for _, token := range tokens {
		c, err := parseConstraint(token)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}

	return constraints, nil
}

func parseConstraint(s string) (VersionConstraint, error) {
	if strings.HasPrefix(s, ">=") {
		v, err := ParseSemVer(s[2:])
		if err != nil {
			return VersionConstraint{}, err
		}
		return VersionConstraint{Op: ">=", Version: v}, nil
	}
	if strings.HasPrefix(s, ">") {
		v, err := ParseSemVer(s[1:])
		if err != nil {
			return VersionConstraint{}, err
		}
		return VersionConstraint{Op: ">", Version: v}, nil
	}
	if strings.HasPrefix(s, "<=") {
		v, err := ParseSemVer(s[2:])
		if err != nil {
			return VersionConstraint{}, err
		}
		return VersionConstraint{Op: "<=", Version: v}, nil
	}
	if strings.HasPrefix(s, "<") {
		v, err := ParseSemVer(s[1:])
		if err != nil {
			return VersionConstraint{}, err
		}
		return VersionConstraint{Op: "<", Version: v}, nil
	}
	if strings.HasPrefix(s, "~") {
		v, err := ParseSemVer(s[1:])
		if err != nil {
			return VersionConstraint{}, err
		}
		return VersionConstraint{Op: "~", Version: v}, nil
	}
	if strings.HasPrefix(s, "^") {
		v, err := ParseSemVer(s[1:])
		if err != nil {
			return VersionConstraint{}, err
		}
		return VersionConstraint{Op: "^", Version: v}, nil
	}

	// 연산자 없으면 exact match
	v, err := ParseSemVer(s)
	if err != nil {
		return VersionConstraint{}, fmt.Errorf("unknown version constraint: %q", s)
	}
	return VersionConstraint{Op: "=", Version: v}, nil
}

// CheckVersion 버전이 제약 조건을 만족하는지 확인
func CheckVersion(version SemVer, constraints []VersionConstraint) bool {
	for _, c := range constraints {
		if !matchConstraint(version, c) {
			return false
		}
	}
	return true
}

func matchConstraint(v SemVer, c VersionConstraint) bool {
	cmp := v.Compare(c.Version)

	switch c.Op {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	case "=":
		return cmp == 0
	case "~":
		// ~1.2.0 → >=1.2.0 <1.3.0 (패치만 허용)
		if cmp < 0 {
			return false
		}
		return v.Major == c.Version.Major && v.Minor == c.Version.Minor
	case "^":
		// ^1.2.0 → >=1.2.0 <2.0.0 (마이너+패치 허용)
		// ^0.2.0 → >=0.2.0 <0.3.0 (major가 0이면 마이너가 범위)
		if cmp < 0 {
			return false
		}
		if c.Version.Major == 0 {
			return v.Major == 0 && v.Minor == c.Version.Minor
		}
		return v.Major == c.Version.Major
	}
	return false
}

// CheckVersionRange 버전 범위 문자열로 직접 검증
func CheckVersionRange(version, rangeStr string) error {
	v, err := ParseSemVer(version)
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", version, err)
	}

	constraints, err := ParseVersionRange(rangeStr)
	if err != nil {
		return fmt.Errorf("invalid version range %q: %w", rangeStr, err)
	}

	if !CheckVersion(v, constraints) {
		return fmt.Errorf("version %s does not satisfy constraint %q", version, rangeStr)
	}

	return nil
}
