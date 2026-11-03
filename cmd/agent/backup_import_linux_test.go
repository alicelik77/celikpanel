//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

func importChunks(t *testing.T, agent *Agent, scope backupScope, name, importID string, data []byte, chunk int, sum string) (backupspec.ImportChunkResponse, error) {
	t.Helper()
	var resp backupspec.ImportChunkResponse
	for offset := 0; ; offset += chunk {
		end := offset + chunk
		if end > len(data) {
			end = len(data)
		}
		req := &backupspec.ImportChunkRequest{
			ProtocolVersion: scope.ProtocolVersion, SubscriptionID: scope.SubscriptionID, DomainID: scope.DomainID,
			DomainName: scope.DomainName, BackupName: name, ImportID: importID,
			Offset: int64(offset), Data: data[offset:end], Final: end == len(data),
			Size: int64(len(data)), SHA256: sum,
		}
		resp = backupspec.ImportChunkResponse{}
		if err := agent.ImportBackupChunk(req, &resp); err != nil {
			return resp, err
		}
		if !resp.Success {
			return resp, nil
		}
		// The same chunk again (a lost answer) is accepted without a second write.
		if offset == 0 && !req.Final {
			again := backupspec.ImportChunkResponse{}
			if err := agent.ImportBackupChunk(req, &again); err != nil || !again.Success || again.Offset != int64(end) {
				t.Fatalf("repeated chunk: %+v %v", again, err)
			}
		}
		if req.Final {
			return resp, nil
		}
	}
}

// A package that went to a remote destination and came back is published
// under its name only after its checksum, scope and payloads are verified,
// and then the existing restore path can use it.
func TestBackupImportPublishesVerifiedPackage(t *testing.T) {
	backupRoot, docroot := installBackupTestPaths(t)
	if err := os.WriteFile(filepath.Join(docroot, "index.html"), []byte(strings.Repeat("site ", 2000)), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	if err := os.Remove(info.Path); err != nil {
		t.Fatal(err)
	}
	scope := testScope()

	// Damaged on the way: refused, nothing published, nothing staged.
	damaged := append([]byte(nil), data...)
	damaged[len(damaged)/2] ^= 0xff
	resp, err := importChunks(t, agent, scope, info.Name, "import-1", damaged, 1000, hexSum)
	if err != nil || resp.Success {
		t.Fatalf("damaged package accepted: %+v %v", resp, err)
	}
	// Another domain's scope: refused by the manifest check.
	other := scope
	other.DomainID = 10
	resp, err = importChunks(t, agent, other, info.Name, "import-2", data, 1000, hexSum)
	if err != nil || resp.Success {
		t.Fatalf("package of another domain accepted: %+v %v", resp, err)
	}
	resp, err = importChunks(t, agent, scope, info.Name, "import-3", data, 1000, hexSum)
	if err != nil || !resp.Success || resp.Backup.Name != info.Name || !resp.Backup.Restorable {
		t.Fatalf("import: %+v %v", resp, err)
	}
	entries, _ := os.ReadDir(filepath.Join(backupRoot, "subscriptions", "7", "domains", "9"))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".import-") {
			t.Fatalf("staging file left: %s", entry.Name())
		}
	}
	// Importing the same package again is the same backup.
	resp, err = importChunks(t, agent, scope, info.Name, "import-4", data, 4096, hexSum)
	if err != nil || !resp.Success {
		t.Fatalf("re-import of identical package: %+v %v", resp, err)
	}
	if err := os.Remove(filepath.Join(docroot, "index.html")); err != nil {
		t.Fatal(err)
	}
	restore := &backupspec.RestoreResponse{}
	if err := agent.restoreBackup(&backupspec.RestoreRequest{ProtocolVersion: backupspec.ProtocolVersion,
		SubscriptionID: 7, DomainID: 9, DomainName: "example.test", BackupName: info.Name}, restore); err != nil || !restore.Success {
		t.Fatalf("restore of imported package: %+v %v", restore, err)
	}
	if _, err := os.Stat(filepath.Join(docroot, "index.html")); err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	// A name that does not match the contents is refused.
	wrongName := strings.Replace(info.Name, "files-", "full-", 1)
	resp, err = importChunks(t, agent, scope, wrongName, "import-5", data, 4096, hexSum)
	if err != nil || resp.Success {
		t.Fatalf("mismatched name accepted: %+v %v", resp, err)
	}
}
