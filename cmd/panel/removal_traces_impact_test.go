package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108 (D-038 §13): the dialog's traces are the owner's; CelikPanel's own
// site vhosts and pools, already named as the sites on it, are dropped.
func TestRemovalTracesKeepOnlyWhatCelikPanelDoesNotKnow(t *testing.T) {
	panel, _, _, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "8.4")
	if _, err := panel.db.GetDB().Exec(`UPDATE sites SET php_fpm_socket = '/var/run/php/php8.4-fpm-site9203.sock' WHERE id = 9203`); err != nil {
		t.Fatal(err)
	}
	preview := transport.ServiceRemovalPreviewResponse{State: "checked", TracesState: "checked", Traces: []transport.RemovalTrace{
		{Kind: "web_config", Path: "/etc/nginx/sites-enabled/php.example.conf", Line: 12, Endpoint: "/run/php/php8.4-fpm-site9203.sock"},
		{Kind: "web_config", Path: "/etc/nginx/sites-enabled/legacy.conf", Line: 5, Endpoint: "/run/php/php8.4-fpm.sock"},
		{Kind: "process", Process: "nginx", PID: 80, Endpoint: "/run/php/php8.4-fpm-site9203.sock"},
		{Kind: "unit", Unit: "billing.service", Relation: "Requires"},
	}}
	simulation := removalSimulationFrom("php-fpm", preview, nil)
	if simulation.Refuses || simulation.TracesState != "checked" || len(simulation.Traces) != 4 {
		t.Fatalf("simulation = %+v", simulation)
	}
	kept := panel.unknownRemovalTraces(context.Background(), simulation.Traces)
	var got []string
	for _, trace := range kept {
		got = append(got, trace.Kind+":"+trace.Path+trace.Unit)
	}
	if strings.Join(got, ",") != "web_config:/etc/nginx/sites-enabled/legacy.conf,unit:billing.service" {
		t.Fatalf("kept = %v", got)
	}
}
