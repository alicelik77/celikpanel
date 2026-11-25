package sitesandbox

import "testing"

// TestASlicePeakAtItsLimitIsAReachedProcessLimit (alpha.149, the alpha.148
// lab's D3, decision 5): on a kernel without pids.events.local (Ubuntu
// 24.04's 6.8) the unit's counters went with its cgroup when the program
// restarted; the slice's own pids.peak reaching its pids.max still says that
// the site's programs reached their process limit since the slice was made.
func TestASlicePeakAtItsLimitIsAReachedProcessLimit(t *testing.T) {
	for _, c := range []struct {
		peak, max string
		want      bool
	}{
		{"64\n", "64\n", true},
		{"65\n", "64\n", true},
		{"63\n", "64\n", false},
		{"64\n", "max\n", false},
		{"", "64\n", false},
	} {
		got := ParseSliceCounters(SliceCgroupFiles{SlicePidsPeak: c.peak, SlicePidsMax: c.max})
		if got.TasksPeakAtLimit != c.want || got.TasksRule != "units" {
			t.Fatalf("peak %q max %q: %+v", c.peak, c.max, got)
		}
	}
	// Where the kernel has pids.events.local its counter is the proof.
	local := ParseSliceCounters(SliceCgroupFiles{HasPidsEventsLocal: true, PidsEventsLocal: "max 0\n", SlicePidsPeak: "64", SlicePidsMax: "64"})
	if local.TasksPeakAtLimit {
		t.Fatal("the peak was read where pids.events.local counts")
	}
}
