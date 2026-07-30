package driver

import "testing"

func TestMaxIterationsDefaultsUnlimitedAndPositiveCaps(t *testing.T) {
	if got := (&DriverSpec{}).maxIterations(); got != 0 {
		t.Fatalf("omitted max_iterations = %d, want unlimited (0)", got)
	}
	if got := (&DriverSpec{MaxIterations: 7}).maxIterations(); got != 7 {
		t.Fatalf("explicit max_iterations = %d, want 7", got)
	}
}
