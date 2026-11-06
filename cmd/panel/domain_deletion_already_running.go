package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// A delete press for a domain whose deletion already continues by itself
// (live incident 6 Nov 2026, alpha.125). The owner pressed delete several
// times on a joined server while the main DNS server was confirming the
// removal with its backup; every press ran the whole deletion again (site,
// mail and DKIM stages, the remote DNS removal re-sent) and answered with
// the same pending text as if something had gone wrong. A press now
// answers at once that the domain is already being deleted and starts
// nothing: the running request, or the joined server's own re-check
// (remote_dns_deletion_resume.go), carries the deletion to its end.
// "Retry this deletion" is still a full retry whenever a person must act
// (a recorded stage failure, a refusal by the main DNS server, a deletion
// that never reached its DNS stage, or any server that is not joined).
//
// Silme zaten kendiliğinden sürüyorsa yeni basış hiçbir şey başlatmaz;
// "bu alan adı zaten siliniyor" yanıtı verir.

const domainDeletionAlreadyRunningReason = "deletion_already_running"

const domainDeletionAlreadyRunningEnglish = "This domain is already being deleted. Nobody needs to do anything: the deletion continues by itself and this page updates by itself."

func writeDomainDeletionAlreadyRunning(w http.ResponseWriter, domain string) {
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  domainDeletionPendingStatus,
		"domain":  domain,
		"stage":   "dns_cleanup",
		"reason":  domainDeletionAlreadyRunningReason,
		"message": domainDeletionAlreadyRunningEnglish,
	})
}

// domainDeletionContinuesByItself reports whether a joined server's
// deletion of this domain reached its DNS stage and waits only for the main
// DNS server, which this server asks again by itself. It only reads. The
// caller holds the domain's operation lock.
func (p *Panel) domainDeletionContinuesByItself(ctx context.Context, domainID int) (bool, string, error) {
	var name, status, mode string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.name, d.status, d.dns_management
		FROM domains d
		JOIN domain_deletion_operations op ON op.domain_id = d.id
		WHERE d.id = ?`, domainID).Scan(&name, &status, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if status != domainDeletionLedgerStatus || mode != setupDNSModeExisting {
		return false, "", nil
	}
	// A refusal by the main DNS server needs a person: the press is the
	// owner's retry after fixing the connection.
	if p.remoteDNSDeletions().lastReason(domainID) == remoteDNSDeletionRefusedReason {
		return false, "", nil
	}
	tx, err := p.db.GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback()
	_, failed, err := readDomainDeletionFailure(ctx, tx, domainID, name)
	if err != nil || failed {
		return false, "", err
	}
	state, err := readRemoteDomainDeletionDNS(ctx, tx, domainID, name)
	if err != nil {
		return false, "", err
	}
	return state.Reached && !state.Done, name, nil
}
