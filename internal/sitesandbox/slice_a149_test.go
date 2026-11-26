package sitesandbox

import "testing"

// TestASlicePeakIsReadWithItsLimit (alpha.149, the alpha.148 lab's D3; fix
// round F7): on a kernel without pids.events.local (Ubuntu 24.04's 6.8) the
// unit's counters went with its cgroup when the program restarted; the
// slice's own pids.peak is carried with the pids.max it was read with, so
// the Panel can compare readings under an unchanged limit (a high-water mark
// alone never says when it was reached). No limit, or a peak that cannot be
// read, carries neither.
func TestASlicePeakIsReadWithItsLimit(t *testing.T) {
	for _, c := range []struct {
		peak, max      string
		wantPeak, want int64
	}{
		{"64\n", "64\n", 64, 64},
		{"50\n", "40\n", 50, 40},
		{"63\n", "64\n", 63, 64},
		{"64\n", "max\n", 0, 0},
		{"", "64\n", 0, 0},
	} {
		got := ParseSliceCounters(SliceCgroupFiles{SlicePidsPeak: c.peak, SlicePidsMax: c.max})
		if got.SlicePidsPeak != c.wantPeak || got.SlicePidsMax != c.want || got.TasksRule != "units" {
			t.Fatalf("peak %q max %q: %+v", c.peak, c.max, got)
		}
	}
	// Where the kernel has pids.events.local its counter is the proof.
	local := ParseSliceCounters(SliceCgroupFiles{HasPidsEventsLocal: true, PidsEventsLocal: "max 0\n", SlicePidsPeak: "64", SlicePidsMax: "64"})
	if local.SlicePidsPeak != 0 || local.SlicePidsMax != 0 {
		t.Fatal("the peak was read where pids.events.local counts")
	}
}
