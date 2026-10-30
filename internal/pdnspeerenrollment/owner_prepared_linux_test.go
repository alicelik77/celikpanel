//go:build linux

package pdnspeerenrollment

import (
	"os"
	"strings"
	"testing"
)

// An interrupted automatic enrollment continues with the credential it
// prepared: the same public key is returned, nothing is written, and an
// unknown or malformed credential is refused.
func TestPreparedOwnerPublicReturnsTheSamePreparedKey(t *testing.T) {
	root := ownerWriterRoot(t)
	prepared, err := prepareOwnerAt(root)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := ownerRootPath(root, CredentialDir+"/"+prepared.CredentialID+".key")
	before, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	again, err := preparedOwnerPublicAt(root, prepared.CredentialID)
	if err != nil || again != prepared {
		t.Fatalf("reuse: %+v %v want %+v", again, err, prepared)
	}
	after, err := os.ReadFile(keyPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("reuse changed the credential: %v", err)
	}
	if _, err := preparedOwnerPublicAt(root, strings.Repeat("0", 32)); err == nil {
		t.Fatal("an unknown credential was accepted")
	}
	if _, err := preparedOwnerPublicAt(root, "../"+prepared.CredentialID); err == nil {
		t.Fatal("a malformed credential identifier was accepted")
	}
	if _, err := readAt(root); !IsCode(err, Disabled) {
		t.Fatalf("reuse created an enrollment: %v", err)
	}
}
