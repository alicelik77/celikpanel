package main

import "testing"

// Security release (20 November 2026): nginx passes a Node.js site's public
// visitors to its application port on this server. The Agent does not trust
// the Panel's value: a port of this server's own services (the database web
// tools on 8306, the mail filter's controller, the panel itself) or of the
// service apps' and programs' range is refused before any vhost is written.
func TestNodeVhostRefusesAPortOfTheServersOwnServices(t *testing.T) {
	for _, port := range []int{8306, 11334, 1923, 2083, 3306, 5432, 8891, 21010, 30010} {
		req := validApplyVhostRequest(t)
		req.ProjectType = "node"
		req.AppPort = port
		if _, err := validatedVhostData(&req); err == nil {
			t.Fatalf("a Node.js vhost was accepted for port %d", port)
		}
	}
	req := validApplyVhostRequest(t)
	req.ProjectType = "node"
	req.AppPort = 3001
	if _, err := validatedVhostData(&req); err != nil {
		t.Fatalf("an ordinary Node.js port was refused: %v", err)
	}
}
