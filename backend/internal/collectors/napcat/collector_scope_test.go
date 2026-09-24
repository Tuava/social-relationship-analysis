package napcat

import "testing"

func TestCountDeltaReportsOnlyCurrentRunGrowth(t *testing.T) {
	if got := countDelta(100, 127); got != 27 {
		t.Fatalf("countDelta(100, 127) = %d", got)
	}
	if got := countDelta(100, 100); got != 0 {
		t.Fatalf("countDelta(100, 100) = %d", got)
	}
	if got := countDelta(100, 90); got != 0 {
		t.Fatalf("countDelta must not report a negative count, got %d", got)
	}
}
