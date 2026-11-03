package services

import (
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 (D-044): while the subscription's disk is full, the site's pool
// carries php_admin_flag[file_uploads] = off; every later render keeps it
// (save, switch) until a request turns it on again.
func TestSitePoolUploadsOffFollowsTheDiskAndSurvivesOtherRenders(t *testing.T) {
	path, _ := withSealedPool(t)
	request := sitePoolTestRequest(transport.SitePoolOperationApply)
	request.Uploads = transport.SitePoolUploadsOff
	if result, err := NewPHPPoolManager().ApplySitePool(request); err != nil || result.Outcome != transport.SiteFileOutcomeWritten {
		t.Fatalf("uploads off: %+v %v", result, err)
	}
	if !strings.Contains(readPoolTestFile(t, path), "php_admin_flag[file_uploads] = off") {
		t.Fatal("the pool does not turn uploads off")
	}
	// A save of the pool page (no disk state in the request) keeps it.
	save := sitePoolTestRequest(transport.SitePoolOperationSave)
	save.Values = &transport.SitePoolValues{PM: "dynamic", PMMaxChildren: 7, PMStartServers: 2, PMMinSpareServers: 1, PMMaxSpareServers: 3, PMMaxRequests: 500}
	if _, err := NewPHPPoolManager().ApplySitePool(save); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readPoolTestFile(t, path), "php_admin_flag[file_uploads] = off") {
		t.Fatal("a save dropped the uploads line")
	}
	// A version switch carries it to the new pool.
	if err := os.MkdirAll(poolDirPath("8.4"), 0o755); err != nil {
		t.Fatal(err)
	}
	move := sitePoolTestRequest(transport.SitePoolOperationSwitch)
	move.NewPHPVersion = "8.4"
	result, err := NewPHPPoolManager().ApplySitePool(move)
	if err != nil || !strings.Contains(readPoolTestFile(t, result.Path), "php_admin_flag[file_uploads] = off") {
		t.Fatalf("switch dropped the uploads line: %+v %v", result, err)
	}
	// Under the limit again: the line goes away.
	back := sitePoolTestRequest(transport.SitePoolOperationApply)
	back.PHPVersion = "8.4"
	back.Uploads = transport.SitePoolUploadsOn
	if _, err := NewPHPPoolManager().ApplySitePool(back); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readPoolTestFile(t, result.Path), "file_uploads") {
		t.Fatal("uploads stay off under the limit")
	}
}
