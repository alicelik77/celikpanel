package sitesandbox

import "testing"

// Security release (20 November 2026): a Node.js site's port, which nginx
// passes the public to, is never one of this server's own services, the
// service apps' and programs' range or the configured panel port.
func TestNodePortRefusal(t *testing.T) {
	for _, port := range []int{80, 1023, 1923, 2083, 2084, 3306, 5432, 8306, 8891, 10040, 11334, 21000, 34999, 65536} {
		if NodePortRefusal(port) == "" {
			t.Fatalf("port %d was accepted", port)
		}
	}
	for _, port := range []int{NodeAssignMin, 3500, NodeAssignMax, 8080, 40000} {
		if refusal := NodePortRefusal(port); refusal != "" {
			t.Fatalf("port %d: %s", port, refusal)
		}
	}
	if NodePortRefusal(4100, 4100) == "" {
		t.Fatal("the configured panel port was accepted")
	}
	p := programProfile()
	p.Kind, p.Port, p.ExtraPorts = KindNode, 8306, nil
	if err := p.Validate(); err == nil {
		t.Fatal("a Node.js unit for a server service's port was accepted")
	}
	p.Port = NodeAssignMin
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}
