package serviceapps

import (
	"strings"
	"testing"
)

// TestUnitKeepsItsSitesSlice: a service app's unit a later release placed in
// its site's slice is written again with that slice and nothing else
// changed; without InSlice it is exactly today's text.
func TestUnitKeepsItsSitesSlice(t *testing.T) {
	p := n8nParams(t)
	before, err := RenderUnit(p)
	if err != nil || !strings.Contains(before, "\nSlice=system.slice\n") {
		t.Fatalf("system slice: %v\n%s", err, before)
	}
	p.SiteID = 9
	if same, err := RenderUnit(p); err != nil || same != before {
		t.Fatalf("a site identity alone changed the unit: %v", err)
	}
	p.InSlice = true
	kept, err := RenderUnit(p)
	if err != nil || !strings.Contains(kept, "\nSlice=celiksite_9.slice\n") || strings.Contains(kept, "Slice=system.slice") {
		t.Fatalf("kept slice: %v\n%s", err, kept)
	}
	_, keptBody, _ := strings.Cut(kept, "\n")
	_, beforeBody, _ := strings.Cut(before, "\n")
	if strings.Replace(keptBody, "\nSlice=celiksite_9.slice\n", "\nSlice=system.slice\n", 1) != beforeBody {
		t.Fatal("keeping the slice changed another line")
	}
	if UnitTextState(kept) != "ours" {
		t.Fatalf("the kept text is not sealed as CelikPanel's own: %s", UnitTextState(kept))
	}
}
