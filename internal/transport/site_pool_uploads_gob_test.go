package transport

import (
	"bytes"
	"encoding/gob"
	"testing"
)

// The pool's uploads state crosses net/rpc's gob intact in every value,
// including "on" (alpha.118 lab, D4: a pointer to false arrived as nil).
func TestSitePoolUploadsSurvivesGob(t *testing.T) {
	for _, want := range []string{SitePoolUploadsKeep, SitePoolUploadsOff, SitePoolUploadsOn} {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(SitePoolRequest{SiteID: 2, Uploads: want}); err != nil {
			t.Fatal(err)
		}
		var got SitePoolRequest
		if err := gob.NewDecoder(&buf).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Uploads != want {
			t.Fatalf("uploads %q arrived as %q", want, got.Uploads)
		}
	}
	if SitePoolUploadsFor(false) != SitePoolUploadsOn || SitePoolUploadsFor(true) != SitePoolUploadsOff {
		t.Fatal("SitePoolUploadsFor")
	}
}
