package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G8): the copy refuses what it must never do before it touches
// anything: copy a site onto itself, an invalid or the same database.
func TestSiteCopyRefusesBeforeTouchingAnything(t *testing.T) {
	site := func(sub, domain int, name string) transport.WordPressSite {
		return transport.WordPressSite{SiteID: domain, SubscriptionID: sub, DomainID: domain, Domain: name, Username: services.SiteUsername(name)}
	}
	op := strings.Repeat("a", 32)
	cases := []struct {
		name string
		req  transport.SiteCopyRequest
		want string
	}{
		{"no operation", transport.SiteCopyRequest{Source: site(1, 2, "a.example"), Target: site(1, 3, "b.example")}, "operation identity"},
		{"onto itself", transport.SiteCopyRequest{OperationID: op, Source: site(1, 2, "a.example"), Target: site(1, 2, "a.example")}, "another site"},
		{"bad database", transport.SiteCopyRequest{OperationID: op, Source: site(1, 2, "a.example"), Target: site(1, 3, "b.example"), SourceDatabase: "x; drop", TargetDatabase: "y"}, "invalid database"},
		{"same database", transport.SiteCopyRequest{OperationID: op, Source: site(1, 2, "a.example"), Target: site(1, 3, "b.example"), SourceDatabase: "db1", TargetDatabase: "db1"}, "new one"},
	}
	for _, c := range cases {
		var resp transport.SiteCopyResponse
		_ = (&Agent{}).SiteCopyV1(&c.req, &resp)
		if resp.Copied || !strings.Contains(resp.Error, c.want) || !resp.CompensationSafe && c.name != "no operation" {
			t.Errorf("%s: %+v", c.name, resp)
		}
	}
}
