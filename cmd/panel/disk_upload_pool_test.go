package main

import (
	"context"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// uploadPoolAgent is the site-file test Agent that also answers the file
// access (FTP/SFTP read-only) call of the upload block.
type uploadPoolAgent struct {
	otherSiteFileAgent
	readOnly []bool
}

func (a *uploadPoolAgent) SetFileAccessReadOnlyV1(request *transport.SetFileAccessReadOnlyRequest, response *transport.SetFileAccessReadOnlyResponse) error {
	a.mu.Lock()
	a.readOnly = append(a.readOnly, request.ReadOnly)
	a.mu.Unlock()
	response.Applied = true
	return nil
}

// TestUploadUnblockReachesThePoolOverRPC (alpha.118 lab, D4): after the disk
// limit was lifted the database said "uploads open" and SFTP reopened, but
// the PHP pool kept php_admin_flag[file_uploads] = off. The request carried
// a pointer to false, which net/rpc's gob does not transmit, so the Agent
// read "keep what the pool says". The request now crosses the real RPC
// transport here, and the Agent must receive "on" after the unblock and
// "off" while full.
func TestUploadUnblockReachesThePoolOverRPC(t *testing.T) {
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, "uploads.example.test", "php")
	if _, err := panel.db.GetDB().Exec(`UPDATE sites SET php_version = '8.3' WHERE domain_id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	agent := &uploadPoolAgent{}
	agent.poolApply = func(request *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
		file := fileResult("uploads.example.test", transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeWritten)
		file.Kind = transport.SiteFileKindPHPPool
		response.File = &file
	}
	attachStartupVhostBatchAgent(t, panel, agent)
	db := panel.db.GetDB()
	ctx := context.Background()

	// Full: blocked, not yet applied.
	if _, err := db.Exec(`INSERT INTO subscription_disk_usage (subscription_id, state, measured_at, upload_blocked, upload_applied)
		VALUES (?, 'measured', '2026-11-03 14:20:27', 1, 0)`, subscriptionID); err != nil {
		t.Fatal(err)
	}
	if err := panel.applyUploadBlocks(ctx); err != nil {
		t.Fatalf("block: %v", err)
	}
	requests := agent.poolRequestList()
	if len(requests) != 1 || requests[0].Uploads != transport.SitePoolUploadsOff {
		t.Fatalf("while full the pool must be told off: %+v", requests)
	}

	// The limit is lifted: the next round turns uploads on again.
	if _, err := db.Exec(`UPDATE subscription_disk_usage SET upload_blocked = 0 WHERE subscription_id = ?`, subscriptionID); err != nil {
		t.Fatal(err)
	}
	if err := panel.applyUploadBlocks(ctx); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	requests = agent.poolRequestList()
	if len(requests) != 2 || requests[1].Uploads != transport.SitePoolUploadsOn {
		t.Fatalf("after the limit is lifted the pool must be told on (not keep): %+v", requests)
	}
	var applied int
	if err := db.QueryRow(`SELECT upload_applied FROM subscription_disk_usage WHERE subscription_id = ?`, subscriptionID).Scan(&applied); err != nil || applied != 0 {
		t.Fatalf("upload_applied = %d, %v", applied, err)
	}
	// A round with nothing to change starts no second mutation.
	if err := panel.applyUploadBlocks(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(agent.poolRequestList()); got != 2 {
		t.Fatalf("an idle round sent %d pool requests, want 2", got)
	}
	agent.mu.Lock()
	readOnly := append([]bool(nil), agent.readOnly...)
	agent.mu.Unlock()
	if len(readOnly) != 2 || !readOnly[0] || readOnly[1] {
		t.Fatalf("file access calls: %v", readOnly)
	}
}
