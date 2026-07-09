package semver

import "testing"

// ordered is a STRICTLY increasing sequence by semver 2.0.0 precedence
// (spec §11). Getting the pre-release ordering right is the whole point:
//   - numeric identifiers compare numerically (beta.2 < beta.11, not lexically)
//   - numeric identifiers have LOWER precedence than alphanumeric (alpha.1 < alpha.beta)
//   - a larger set of pre-release fields wins when prefixes are equal (alpha < alpha.1)
//   - any pre-release has lower precedence than the release (1.0.0-rc.1 < 1.0.0)
var ordered = []string{
	"1.0.0-alpha",
	"1.0.0-alpha.1",
	"1.0.0-alpha.beta",
	"1.0.0-beta",
	"1.0.0-beta.2",
	"1.0.0-beta.11",
	"1.0.0-rc.1",
	"1.0.0",
	"1.0.1",
	"1.1.0",
	"2.0.0",
	"2.1.0",
	"2.1.1",
}

func TestCompareTotalOrdering(t *testing.T) {
	for i := range ordered {
		for j := range ordered {
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if got := Compare(ordered[i], ordered[j]); got != want {
				t.Errorf("Compare(%q, %q) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}

func TestBuildMetadataIgnored(t *testing.T) {
	// Build metadata MUST NOT figure into precedence (spec §10 / §11).
	cases := [][2]string{
		{"1.0.0+build.1", "1.0.0+build.2"},
		{"1.0.0-alpha+x", "1.0.0-alpha+y"},
		{"2.1.1+20130313144700", "2.1.1"},
	}
	for _, c := range cases {
		if got := Compare(c[0], c[1]); got != 0 {
			t.Errorf("Compare(%q, %q) = %d, want 0 (build metadata ignored)", c[0], c[1], got)
		}
	}
}
