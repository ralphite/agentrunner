package tool

import "testing"

func TestParsePSTableKeepsOnlyInitParented(t *testing.T) {
	out := "  101     1   101\n" + // orphan: kept
		"  202   100   202\n" + // live parent: dropped
		"    1     0     1\n" + // init itself: dropped
		"garbage line\n" +
		"  303     1     0\n" // pgid 0: dropped
	procs := parsePSTable(out)
	if len(procs) != 1 || procs[0].pid != 101 || procs[0].pgid != 101 {
		t.Fatalf("parsePSTable = %+v, want exactly pid 101", procs)
	}
}
