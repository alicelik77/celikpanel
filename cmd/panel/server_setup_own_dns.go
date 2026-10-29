package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The setup's own DNS software is not "existing DNS" (Beta-1 re-run P9,
// 2026-10-29). After the P4 rejoin the backup's new review was refused with
// server_setup_existing_dns_requires_migration because of the PowerDNS its
// own earlier run had installed.
//
// An active engine is this setup's own when this panel recorded installing
// it: a committed first install (no source engine) whose request is the
// succeeded DNS step of one of this panel's setup runs, at the engine's
// current epoch (nothing switched it since), with no switch attached and no
// operation marker pending. Only an engine without that evidence is
// "existing DNS that needs migration". The run's DNS step adopts an own
// engine that matches its plan without installing anything
// (startServerSetupDNS); one that does not match is said as such
// (server_setup_dns_installed_differs).
//
// Kurulumun kendi kurduğu ve kaydettiği DNS yazılımı taşıma gerektiren mevcut
// DNS sayılmaz.

const serverSetupDNSInstalledDiffersCode = "server_setup_dns_installed_differs"

// serverSetupDNSInstalledDiffersFailure is that state at the DNS step: the
// setup's own engine serves another second server or other names than the
// bound plan (D-024: who = owner; next = review a revised plan that keeps
// them, or change the DNS pair in Server → DNS; then setup continues).
func serverSetupDNSInstalledDiffersFailure() error {
	return &serverSetupChildFailure{Code: serverSetupDNSInstalledDiffersCode, Message: "This server already runs the DNS software an earlier run of this setup installed, paired with another second server or other names than this run's join. Setup never replaces it, and nothing was changed. You, the owner: choose Review a revised plan and keep the earlier pair, or change the DNS pair first in Server → DNS; setup then continues."}
}

func (p *Panel) serverSetupOwnDNSEngine(ctx context.Context, state dnsEngineDBState) (bool, error) {
	if state.ActiveEngine == "" || state.CurrentSwitchID != "" {
		return false, nil
	}
	marker, err := readDNSEngineOperationMarker(ctx, p.db.GetDB())
	if err != nil || marker != nil {
		return false, err
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT execution_json FROM server_setup_executions`)
	if err != nil {
		return false, err
	}
	var requests []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return false, err
		}
		var execution serverSetupExecution
		if json.Unmarshal([]byte(raw), &execution) != nil {
			continue
		}
		for _, step := range execution.Steps {
			if step.Kind == "dns" && step.Status == "succeeded" && validServiceOperationID(step.RequestID) {
				requests = append(requests, step.RequestID)
			}
		}
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, request := range requests {
		persisted, err := readDNSEngineSwitchByRequest(ctx, p.db.GetDB(), request)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, err
		}
		if persisted.Phase == "committed" && persisted.SourceEngine == "" && persisted.TargetEngine == state.ActiveEngine && persisted.TargetEpoch == state.EngineEpoch {
			return true, nil
		}
	}
	return false, nil
}

// serverSetupOwnDNSMatchesJoinDraft: an own engine and a plan that still
// waits for its second server agree on everything this server decides: the
// engine, the pair role and this server's own address and name. The second
// server's address is compared at its DNS step, once it is bound.
func serverSetupOwnDNSMatchesJoinDraft(d serverSetupDraft, state dnsEngineDBState) bool {
	localNS := d.NS1
	if d.DNSRole == transport.DNSPairRoleSecondary {
		localNS = d.NS2
	}
	local, _ := canonicalIPv4(d.LocalIP)
	return state.ActiveEngine == transport.DNSEngine(d.DNSEngine) && state.Topology == transport.DNSTopologyPaired &&
		state.PairRole == d.DNSRole && state.LocalIP == local && state.LocalNS == localNS
}
