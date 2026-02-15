package plugin

import (
	"testing"
)

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input string
		want  SemVer
		ok    bool
	}{
		{"1.0.0", SemVer{1, 0, 0}, true},
		{"2.1.3", SemVer{2, 1, 3}, true},
		{"0.0.1", SemVer{0, 0, 1}, true},
		{"1.0.0-beta", SemVer{1, 0, 0}, true},
		{"1.0.0+build.123", SemVer{1, 0, 0}, true},
		{"1.0", SemVer{}, false},
		{"abc", SemVer{}, false},
		{"", SemVer{}, false},
	}

	for _, tc := range tests {
		got, err := ParseSemVer(tc.input)
		if tc.ok && err != nil {
			t.Errorf("ParseSemVer(%q) unexpected error: %v", tc.input, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("ParseSemVer(%q) expected error, got %v", tc.input, got)
		}
		if tc.ok && got != tc.want {
			t.Errorf("ParseSemVer(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestSemVer_Compare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.1.0", "1.0.9", 1},
	}

	for _, tc := range tests {
		a, _ := ParseSemVer(tc.a)
		b, _ := ParseSemVer(tc.b)
		got := a.Compare(b)
		if got != tc.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheckVersionRange_GTE(t *testing.T) {
	// >=1.0.0
	if err := CheckVersionRange("1.0.0", ">=1.0.0"); err != nil {
		t.Errorf("1.0.0 should satisfy >=1.0.0: %v", err)
	}
	if err := CheckVersionRange("2.0.0", ">=1.0.0"); err != nil {
		t.Errorf("2.0.0 should satisfy >=1.0.0: %v", err)
	}
	if err := CheckVersionRange("0.9.9", ">=1.0.0"); err == nil {
		t.Error("0.9.9 should NOT satisfy >=1.0.0")
	}
}

func TestCheckVersionRange_Compound(t *testing.T) {
	// >=1.0.0 <2.0.0
	constraint := ">=1.0.0 <2.0.0"

	good := []string{"1.0.0", "1.5.0", "1.9.9"}
	for _, v := range good {
		if err := CheckVersionRange(v, constraint); err != nil {
			t.Errorf("%s should satisfy %q: %v", v, constraint, err)
		}
	}

	bad := []string{"0.9.9", "2.0.0", "3.0.0"}
	for _, v := range bad {
		if err := CheckVersionRange(v, constraint); err == nil {
			t.Errorf("%s should NOT satisfy %q", v, constraint)
		}
	}
}

func TestCheckVersionRange_Tilde(t *testing.T) {
	// ~1.2.0 → >=1.2.0, same major.minor
	constraint := "~1.2.0"

	good := []string{"1.2.0", "1.2.5", "1.2.99"}
	for _, v := range good {
		if err := CheckVersionRange(v, constraint); err != nil {
			t.Errorf("%s should satisfy %q: %v", v, constraint, err)
		}
	}

	bad := []string{"1.1.9", "1.3.0", "2.0.0"}
	for _, v := range bad {
		if err := CheckVersionRange(v, constraint); err == nil {
			t.Errorf("%s should NOT satisfy %q", v, constraint)
		}
	}
}

func TestCheckVersionRange_Caret(t *testing.T) {
	// ^1.2.0 → >=1.2.0, same major
	constraint := "^1.2.0"

	good := []string{"1.2.0", "1.3.0", "1.99.99"}
	for _, v := range good {
		if err := CheckVersionRange(v, constraint); err != nil {
			t.Errorf("%s should satisfy %q: %v", v, constraint, err)
		}
	}

	bad := []string{"1.1.9", "2.0.0", "0.9.0"}
	for _, v := range bad {
		if err := CheckVersionRange(v, constraint); err == nil {
			t.Errorf("%s should NOT satisfy %q", v, constraint)
		}
	}
}

func TestCheckVersionRange_CaretZero(t *testing.T) {
	// ^0.2.0 → >=0.2.0, same 0.minor
	constraint := "^0.2.0"

	good := []string{"0.2.0", "0.2.5"}
	for _, v := range good {
		if err := CheckVersionRange(v, constraint); err != nil {
			t.Errorf("%s should satisfy %q: %v", v, constraint, err)
		}
	}

	bad := []string{"0.1.9", "0.3.0", "1.0.0"}
	for _, v := range bad {
		if err := CheckVersionRange(v, constraint); err == nil {
			t.Errorf("%s should NOT satisfy %q", v, constraint)
		}
	}
}

func TestCheckVersionRange_ExactMatch(t *testing.T) {
	if err := CheckVersionRange("1.0.0", "1.0.0"); err != nil {
		t.Errorf("exact match should pass: %v", err)
	}
	if err := CheckVersionRange("1.0.1", "1.0.0"); err == nil {
		t.Error("1.0.1 should NOT match exact 1.0.0")
	}
}

func TestCheckVersionRange_InvalidInputs(t *testing.T) {
	if err := CheckVersionRange("bad", ">=1.0.0"); err == nil {
		t.Error("expected error for bad version")
	}
	if err := CheckVersionRange("1.0.0", ""); err == nil {
		t.Error("expected error for empty range")
	}
	if err := CheckVersionRange("1.0.0", "???"); err == nil {
		t.Error("expected error for invalid range")
	}
}
