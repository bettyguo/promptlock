package version

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in       string
		want     string // canonical String() form
		wantFail bool
	}{
		{"1.0.0", "1.0.0", false},
		{"0.0.1", "0.0.1", false},
		{"10.20.30", "10.20.30", false},
		{"1.4.0-rc.1", "1.4.0-rc.1", false},
		{"1.0.0-alpha+exp.sha.5114f85", "1.0.0-alpha+exp.sha.5114f85", false},
		{"1.0.0+20130313144700", "1.0.0+20130313144700", false},

		// Failures
		{"", "", true},
		{"1", "", true},
		{"1.2", "", true},
		{"1.2.3.4", "", true},
		{"01.2.3", "", true},   // leading zero
		{"1.2.-3", "", true},   // negative
		{"1.2.3-", "", true},   // empty pre-release
		{"1.2.3+", "", true},   // empty build
		{"1.2.3-+x", "", true}, // empty pre-release with build
		{"a.b.c", "", true},
		{"1.0.0-01", "", true}, // leading zero pre-release numeric
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			v, err := Parse(c.in)
			if c.wantFail {
				if err == nil {
					t.Fatalf("Parse(%q) = %v, want error", c.in, v)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.in, err)
			}
			if got := v.String(); got != c.want {
				t.Fatalf("Parse(%q).String() = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.99", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0+meta", "1.0.0", 0}, // build metadata ignored
	}
	for _, c := range cases {
		va, _ := Parse(c.a)
		vb, _ := Parse(c.b)
		got := va.Compare(vb)
		if got != c.want {
			t.Fatalf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestBump(t *testing.T) {
	v, _ := Parse("1.4.2-rc.1+meta")
	if got := v.BumpMajor().String(); got != "2.0.0" {
		t.Errorf("BumpMajor = %q, want 2.0.0", got)
	}
	if got := v.BumpMinor().String(); got != "1.5.0" {
		t.Errorf("BumpMinor = %q, want 1.5.0", got)
	}
	if got := v.BumpPatch().String(); got != "1.4.3" {
		t.Errorf("BumpPatch = %q, want 1.4.3", got)
	}
}
