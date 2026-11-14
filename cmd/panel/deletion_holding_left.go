package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail left in place (alpha.135). Before alpha.135 deleting a mailbox left
// its stored mail in the domain's live mail folder, and a mailbox made again
// under the same address saw it: a privacy defect when the address is given
// to another person. The deletion now holds the mail (deletion_holding.go);
// what earlier releases left is closed here, with the same holding list:
//
//   - making a mailbox first puts a folder left at its address into the list
//     ("left by an earlier deletion"), then makes the mailbox empty
//     (domain_mail_handlers.go, Agent.AddMailAccount with LeftDomainID);
//   - Maintenance → What deletions left lists the folders still left in live
//     domain folders, and "Keep 30 days…" puts one into the list when the
//     administrator asks (POST .../deletion-holding/left). Nothing is moved
//     at an update or by a read.
//
// The Agent derives the folder from the address and refuses an address that
// has a mailbox account (cmd/agent/deletion_holding_left.go).
//
// Yerinde kalan posta: kutu yeniden açılırken önce 30 gün saklamaya alınır;
// Bakım listesi canlı klasörde kalanları gösterir, yönetici isterse alır.

const holdingLeftPath = "left"

// holdingAgentLeft and holdingAgentHoldLeft are the two Agent calls (tests
// replace them).
var holdingAgentLeft = func(p *Panel, ctx context.Context) (transport.LeftMailFoldersResponse, error) {
	var resp transport.LeftMailFoldersResponse
	err := p.callAgentContext(ctx, "Agent.LeftMailFoldersV1", &transport.LeftMailFoldersRequest{}, &resp)
	return resp, err
}

var holdingAgentHoldLeft = func(p *Panel, ctx context.Context, req transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
	var resp transport.HoldLeftMailFolderResponse
	err := p.callAgentContext(ctx, "Agent.HoldLeftMailFolderV1", &req, &resp)
	return resp, err
}

// leftMailView is one row of the folders still left in place.
type leftMailView struct {
	Address  string `json:"address"`
	Domain   string `json:"domain"`
	DomainID int    `json:"domain_id"`
	// Bytes is -1 when the size could not be read.
	Bytes int64  `json:"bytes"`
	Files int    `json:"files,omitempty"`
	Path  string `json:"path,omitempty"`
}

// leftMailFolders is what the Agent finds left in live domain folders,
// narrowed to domains that exist here, are not being deleted, and addresses
// that have no mailbox. Read only.
func (p *Panel) leftMailFolders(ctx context.Context) ([]leftMailView, error) {
	out := []leftMailView{}
	resp, err := holdingAgentLeft(p, ctx)
	if err != nil {
		return out, err
	}
	if resp.Error != "" {
		return out, errors.New(resp.Error)
	}
	if resp.More {
		log.Printf("[holding] more mail folders are left in place than one answer lists")
	}
	for _, folder := range resp.Folders {
		_, domainName, ok := strings.Cut(folder.Address, "@")
		if !ok {
			continue
		}
		var domainID int
		var name string
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id, name FROM domains WHERE lower(name) = lower(?)`, domainName).Scan(&domainID, &name); err != nil {
			// Not a domain of this panel (or not readable): not offered.
			continue
		}
		if exists, err := p.emailAccountExists(ctx, folder.Address); err != nil || exists {
			continue
		}
		if p.ensureMailDomainMutable(ctx, domainID) != nil {
			// Its domain is being deleted: the deletion takes its folder.
			continue
		}
		out = append(out, leftMailView{Address: folder.Address, Domain: name, DomainID: domainID, Bytes: folder.Bytes, Files: folder.Files, Path: folder.Path})
	}
	return out, nil
}

// holdLeftMail saves the record of a folder the Agent moved into the holding
// folder and answers what the result says about it.
func (p *Panel) holdLeftMail(ctx context.Context, r *http.Request, domainID int, domain, address, name string, bytes int64) *holdingKeptView {
	rec := holdingRecord{ID: holdingMailID(name), Kind: transport.HeldItemMail, Name: name,
		DomainID: domainID, Domain: domain, Mailbox: address, Bytes: bytes}
	kept := p.holdDomainDeletionItem(ctx, r, rec)
	p.auditDetailed(r, "deletion_hold.left:"+rec.ID, "domain", domainID, map[string]any{"mailbox": address, "bytes": bytes})
	return kept
}

// handleHoldLeftMail: POST {"address"} puts the folder left at that address
// into the holding list (administrator only; asked in the panel's own window).
func (p *Panel) handleHoldLeftMail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var request struct {
		Address string `json:"address"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, domainPart, ok := strings.Cut(strings.TrimSpace(request.Address), "@")
	if !ok {
		writeClientError(w, http.StatusBadRequest, "invalid address")
		return
	}
	ctx := r.Context()
	var domainID int
	var domain string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id, name FROM domains WHERE lower(name) = lower(?)`, domainPart).Scan(&domainID, &domain); err != nil {
		writeClientError(w, http.StatusNotFound, "invalid request")
		return
	}
	address, err := transport.CanonicalMailboxForDomain(request.Address, domain)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid address")
		return
	}
	p.mailMutationMu.Lock()
	defer p.mailMutationMu.Unlock()
	if err := p.ensureMailDomainMutable(ctx, domainID); err != nil {
		writeMailDomainMutationError(w, err)
		return
	}
	exists, err := p.emailAccountExists(ctx, address)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if exists {
		writeCodedError(w, http.StatusConflict, "HOLDING_LEFT_IN_USE",
			"This address has a mailbox again, so its folder is in use. Nothing was moved; the list is read again.", "")
		return
	}
	resp, callErr := holdingAgentHoldLeft(p, ctx, transport.HoldLeftMailFolderRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: address, DomainID: domainID,
	})
	var serverErr rpc.ServerError
	switch {
	case callErr != nil && errors.As(callErr, &serverErr):
		resp.Error = callErr.Error()
	case callErr != nil:
		p.auditDetailed(r, "deletion_hold.left.unknown:"+address, "domain", domainID, map[string]any{"mailbox": address, "error": boundedDeletionErrorLine(callErr.Error())})
		writeCodedErrorVars(w, http.StatusBadGateway, "HOLDING_LEFT_UNKNOWN",
			"No answer came, so it is not known whether the folder was moved. The list is read again; if it is still there, press Keep 30 days… again.",
			"", nil, map[string]string{"line": boundedDeletionErrorLine(callErr.Error())})
		return
	}
	if resp.Error != "" {
		line := boundedDeletionErrorLine(resp.Error)
		p.auditDetailed(r, "deletion_hold.left.failed:"+address, "domain", domainID, map[string]any{"mailbox": address, "error": line})
		writeCodedErrorVars(w, http.StatusConflict, "HOLDING_LEFT_FAILED",
			fmt.Sprintf("The folder was not moved; it stays where it is. The server said: %s", line),
			"", nil, map[string]string{"line": line})
		return
	}
	if !resp.Held {
		// Nothing there any more (moved by an earlier press): the list is read again.
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "absent"})
		return
	}
	kept := p.holdLeftMail(ctx, r, domainID, domain, address, resp.HeldName, resp.HeldBytes)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "held", "held": kept})
}
