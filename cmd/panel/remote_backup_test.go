package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/remotebackup"
	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/transport"
)

// ----- fakes -----

type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	failPut []error // returned by the next Put calls, in order
	puts    int
}

func newMemStore() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) Put(_ context.Context, key string, r io.Reader) (remotebackup.PutResult, error) {
	data, err := io.ReadAll(r)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.puts++
	if err != nil {
		return remotebackup.PutResult{}, err
	}
	if len(m.failPut) > 0 {
		f := m.failPut[0]
		m.failPut = m.failPut[1:]
		if f != nil {
			return remotebackup.PutResult{}, f
		}
	}
	m.objects[key] = data
	sum := sha256.Sum256(data)
	return remotebackup.PutResult{Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Verified: "size+sha256"}, nil
}

func (m *memStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[key]
	if !ok {
		return nil, &remotebackup.Error{Kind: remotebackup.KindNotFound, Op: "get", Message: "missing"}
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memStore) Stat(_ context.Context, key string) (remotebackup.Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[key]
	if !ok {
		return remotebackup.Object{}, &remotebackup.Error{Kind: remotebackup.KindNotFound, Op: "stat", Message: "missing"}
	}
	return remotebackup.Object{Key: key, Size: int64(len(data))}, nil
}

func (m *memStore) List(_ context.Context, prefix string) ([]remotebackup.Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []remotebackup.Object{}
	for k, v := range m.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, remotebackup.Object{Key: k, Size: int64(len(v))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *memStore) Close() error { return nil }

type remoteFakeAgent struct {
	mu        sync.Mutex
	content   []byte
	name      string
	creates   []backupspec.CreateRequest
	deletes   []string
	imported  []byte
	finalSum  string
	restores  []string
	aborts    int
	importErr string
}

func (a *remoteFakeAgent) info() backupspec.Info {
	created, _ := remotebackup.BackupCreatedAt(a.name)
	return backupspec.Info{Name: a.name, Size: int64(len(a.content)), Type: "full", Origin: "manual", Restorable: true, CreatedAt: created}
}

func (a *remoteFakeAgent) CreateBackup(req *backupspec.CreateRequest, resp *backupspec.CreateResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.creates = append(a.creates, *req)
	resp.Success, resp.Backup = true, a.info()
	return nil
}

func (a *remoteFakeAgent) InspectBackup(req *backupspec.InspectRequest, resp *backupspec.InspectResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if req.BackupName != a.name {
		resp.Error = "no such backup"
		return nil
	}
	resp.Success, resp.Backup = true, a.info()
	return nil
}

func (a *remoteFakeAgent) ReadBackupChunk(req *backupspec.ReadChunkRequest, resp *backupspec.ReadChunkResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	end := req.Offset + int64(req.MaxBytes)
	if end > int64(len(a.content)) {
		end = int64(len(a.content))
	}
	resp.Data = append([]byte(nil), a.content[req.Offset:end]...)
	resp.Offset, resp.Size, resp.EOF = end, int64(len(a.content)), end == int64(len(a.content))
	return nil
}

func (a *remoteFakeAgent) DeleteBackup(req *backupspec.DeleteRequest, ok *bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deletes = append(a.deletes, req.BackupName)
	*ok = true
	return nil
}

func (a *remoteFakeAgent) ImportBackupChunk(req *backupspec.ImportChunkRequest, resp *backupspec.ImportChunkResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.importErr != "" {
		resp.Error = a.importErr
		return nil
	}
	if req.Offset == 0 && !req.Final {
		a.imported = nil
	}
	if req.Offset != int64(len(a.imported)) {
		resp.Error = "out of order"
		return nil
	}
	a.imported = append(a.imported, req.Data...)
	if req.Final {
		a.finalSum = req.SHA256
	}
	resp.Success, resp.Offset = true, int64(len(a.imported))
	return nil
}

func (a *remoteFakeAgent) AbortBackupImport(_ *backupspec.ImportAbortRequest, ok *bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.aborts++
	*ok = true
	return nil
}

func (a *remoteFakeAgent) RestoreBackup(req *backupspec.RestoreRequest, resp *backupspec.RestoreResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restores = append(a.restores, req.BackupName)
	resp.Success = true
	return nil
}

func (a *remoteFakeAgent) HostPlatform(_ *transport.Empty, response *transport.HostPlatformResponse) error {
	*response = debianPolicyTestIdentity()
	return nil
}

type remoteFixture struct {
	panel                 *Panel
	agent                 *remoteFakeAgent
	store                 *memStore
	adminID, customerID   int
	resellerID            int
	subID, domainID       int
	serverDest, ownDest   int64
	passphrase, secretKey string
}

func newRemoteFixture(t *testing.T) remoteFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	db := database.GetDB()
	mustID := func(query string, args ...any) int {
		res, err := db.Exec(query, args...)
		if err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	f := remoteFixture{passphrase: "correct horse battery staple", secretKey: "very-secret-storage-key-123"}
	f.adminID = mustID(`INSERT INTO users (username,password_hash,email,role) VALUES ('rb-admin','x','a@example.test','admin')`)
	f.resellerID = mustID(`INSERT INTO users (username,password_hash,email,role) VALUES ('rb-reseller','x','r@example.test','reseller')`)
	f.customerID = mustID(`INSERT INTO users (username,password_hash,email,role,parent_id) VALUES ('rb-customer','x','c@example.test','customer',?)`, f.resellerID)
	f.subID = mustID(`INSERT INTO subscriptions (owner_id,name) VALUES (?, 'rb-sub')`, f.customerID)
	f.domainID = mustID(`INSERT INTO domains (subscription_id,name) VALUES (?, 'shop.example')`, f.subID)
	key := make([]byte, 32)
	rand.Read(key)
	box, err := secrets.New(key)
	if err != nil {
		t.Fatal(err)
	}
	f.panel = &Panel{db: database, secrets: box}
	f.agent = &remoteFakeAgent{name: "full-20261102T020000.000000000Z-0123456789abcdef.cpbak"}
	f.agent.content = make([]byte, 3<<20+777)
	rand.Read(f.agent.content)

	server := rpc.NewServer()
	if err := server.RegisterName("Agent", f.agent); err != nil {
		t.Fatal(err)
	}
	attachPanelBackupV2AgentServer(t, f.panel, server)

	f.store = newMemStore()
	oldOpen := openRemoteBackupStore
	openRemoteBackupStore = func(context.Context, string, remoteDestinationConfig, remoteDestinationSecret, string) (remotebackup.Store, error) {
		return f.store, nil
	}
	t.Cleanup(func() { openRemoteBackupStore = oldOpen })
	if err := f.panel.setSetting(context.Background(), settingRemoteBackupServerTag, "panel.example"); err != nil {
		t.Fatal(err)
	}
	f.serverDest = f.insertDestination(t, 0, true)
	f.ownDest = f.insertDestination(t, int64(f.customerID), false)
	return f
}

func (f remoteFixture) insertDestination(t *testing.T, owner int64, offered bool) int64 {
	t.Helper()
	secretJSON, _ := json.Marshal(remoteDestinationSecret{SecretAccessKey: f.secretKey})
	sealedSecret, _ := f.panel.secrets.Encrypt(string(secretJSON))
	sealedPass, _ := f.panel.secrets.Encrypt(f.passphrase)
	cfg, _ := json.Marshal(remoteDestinationConfig{Endpoint: "https://s3.example", Bucket: "b", AccessKeyID: "AKIDTESTKEY1234"})
	var ownerVal any
	if owner > 0 {
		ownerVal = owner
	}
	o := 0
	if offered {
		o = 1
	}
	res, err := f.panel.db.GetDB().Exec(`INSERT INTO remote_backup_destinations
		(owner_user_id, name, kind, config_json, secret_sealed, passphrase_sealed, passphrase_check, offered_to_accounts)
		VALUES (?, 'dest', 's3', ?, ?, ?, 'ABCD-1234', ?)`, ownerVal, string(cfg), sealedSecret, sealedPass, o)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func attachPanelBackupV2AgentServer(t *testing.T, p *Panel, server *rpc.Server) {
	t.Helper()
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

func (f remoteFixture) site(t *testing.T) remoteDomain {
	t.Helper()
	site, err := f.panel.loadRemoteDomain(context.Background(), f.domainID)
	if err != nil {
		t.Fatal(err)
	}
	return site
}

// ----- tests -----

// A backup sent by hand is encrypted, stored with its side file, and comes
// back through download, checks, decryption, the Agent's import and the
// existing restore path.
func TestRemoteBackupUploadAndRestoreRoundTrip(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	id, created, err := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{
		Kind: "upload", DestinationID: f.serverDest, Site: f.site(t), BackupName: f.agent.name, RequestedBy: int64(f.adminID),
	})
	if err != nil || !created {
		t.Fatal(err)
	}
	f.panel.runRemoteBackupQueue(ctx)
	op, err := f.panel.loadRemoteOperation(ctx, id)
	if err != nil || op.State != remoteOpSucceeded {
		t.Fatalf("upload: %+v %v", op, err)
	}
	wantKey := "panel.example/sites/shop.example-d" + strconv.Itoa(f.domainID) + "/manual/" + f.agent.name + ".gpg"
	if op.ObjectKey != wantKey {
		t.Fatalf("key %s want %s", op.ObjectKey, wantKey)
	}
	encrypted := f.store.objects[wantKey]
	if bytes.Contains(encrypted, f.agent.content[:256]) {
		t.Fatal("the stored object is not encrypted")
	}
	plain, err := remotebackup.Decrypt(bytes.NewReader(encrypted), f.passphrase)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(plain)
	if err != nil || !bytes.Equal(got, f.agent.content) {
		t.Fatalf("decrypted object differs: %v", err)
	}
	side, err := remotebackup.ParseSidecar(f.store.objects[wantKey+".json"])
	sum := sha256.Sum256(f.agent.content)
	if err != nil || side.PlainSHA256 != hex.EncodeToString(sum[:]) || side.Domain != "shop.example" || side.PassphraseCheck != "ABCD-1234" {
		t.Fatalf("side file %+v %v", side, err)
	}
	if strings.Contains(string(f.store.objects[wantKey+".json"]), f.passphrase) {
		t.Fatal("passphrase in the side file")
	}
	if len(f.agent.deletes) != 0 {
		t.Fatal("a backup sent by hand was removed locally")
	}

	restoreID, _, err := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{
		Kind: "restore", DestinationID: f.serverDest, Site: f.site(t), BackupName: f.agent.name,
		ObjectKey: wantKey, RequestedBy: int64(f.adminID),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.panel.runRemoteBackupQueue(ctx)
	op, _ = f.panel.loadRemoteOperation(ctx, restoreID)
	if op.State != remoteOpSucceeded || op.Verified != "sha256+mdc" {
		t.Fatalf("restore: %+v", op)
	}
	if !bytes.Equal(f.agent.imported, f.agent.content) || f.agent.finalSum != hex.EncodeToString(sum[:]) {
		t.Fatal("the Agent was not given the exact package")
	}
	if len(f.agent.restores) != 1 || f.agent.restores[0] != f.agent.name {
		t.Fatalf("restore path not used: %v", f.agent.restores)
	}
}

// A damaged object is never imported: the integrity check fails, the import
// is aborted and the restore path is not reached.
func TestRemoteBackupRestoreRefusesDamagedObject(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	id, _, _ := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{Kind: "upload", DestinationID: f.serverDest,
		Site: f.site(t), BackupName: f.agent.name, RequestedBy: int64(f.adminID)})
	f.panel.runRemoteBackupQueue(ctx)
	op, _ := f.panel.loadRemoteOperation(ctx, id)
	obj := f.store.objects[op.ObjectKey]
	obj[len(obj)/2] ^= 0x40
	restoreID, _, _ := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{Kind: "restore", DestinationID: f.serverDest,
		Site: f.site(t), BackupName: f.agent.name, ObjectKey: op.ObjectKey, RequestedBy: int64(f.adminID)})
	f.panel.runRemoteBackupQueue(ctx)
	op, _ = f.panel.loadRemoteOperation(ctx, restoreID)
	if op.State != remoteOpFailed || op.FailureClass != remoteFailureVerified || len(f.agent.restores) != 0 || f.agent.finalSum != "" {
		t.Fatalf("damaged object: %+v restores %v", op, f.agent.restores)
	}
	if f.agent.aborts == 0 {
		t.Fatal("the staged import was not dropped")
	}
}

// An unreachable storage keeps the operation's identity: it is queued again
// with the failure recorded, and the retry finishes the same operation.
func TestRemoteBackupTransientFailureIsRetriedUnderSameID(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	f.store.failPut = []error{&remotebackup.Error{Kind: remotebackup.KindNetwork, Op: "put", Message: "the connection to the storage failed"}}
	id, _, _ := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{Kind: "upload", DestinationID: f.serverDest,
		Site: f.site(t), BackupName: f.agent.name, RequestedBy: int64(f.adminID)})
	f.panel.runRemoteBackupQueue(ctx)
	op, _ := f.panel.loadRemoteOperation(ctx, id)
	if op.State != remoteOpQueued || op.FailureCode != "REMOTE_UNREACHABLE" || op.Attempts != 1 || op.NextAttemptAt == "" {
		t.Fatalf("after a transient failure: %+v", op)
	}
	g := remoteOperationGuidance(op, true)
	if g.Result != "retrying" || g.WhoActs != "panel" {
		t.Fatalf("guidance %+v", g)
	}
	// Not yet due: nothing runs.
	f.panel.runRemoteBackupQueue(ctx)
	if again, _ := f.panel.loadRemoteOperation(ctx, id); again.Attempts != 1 {
		t.Fatal("a retry ran before its time")
	}
	f.panel.db.GetDB().Exec(`UPDATE remote_backup_operations SET next_attempt_at = '2000-01-01T00:00:00Z' WHERE id = ?`, id)
	f.panel.runRemoteBackupQueue(ctx)
	op, _ = f.panel.loadRemoteOperation(ctx, id)
	if op.State != remoteOpSucceeded || op.Attempts != 2 || op.FailureCode != "" {
		t.Fatalf("retry: %+v", op)
	}
}

// Refused credentials are a prerequisite for the destination's owner and
// are not retried by themselves.
func TestRemoteBackupCredentialFailureIsPrerequisite(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	f.store.failPut = []error{&remotebackup.Error{Kind: remotebackup.KindCredentials, Op: "put", Message: "the storage refused the access key or secret (403 InvalidAccessKeyId)"}}
	id, _, _ := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{Kind: "upload", DestinationID: f.ownDest,
		Site: f.site(t), BackupName: f.agent.name, RequestedBy: int64(f.customerID)})
	f.panel.runRemoteBackupQueue(ctx)
	op, _ := f.panel.loadRemoteOperation(ctx, id)
	g := remoteOperationGuidance(op, false)
	if op.State != remoteOpFailed || op.FailureClass != remoteFailurePrerequisite || g.WhoActs != "destination_owner" {
		t.Fatalf("%+v %+v", op, g)
	}
	if strings.Contains(op.FailureDetail, f.secretKey) {
		t.Fatal("secret in the failure detail")
	}
}

// The same request, or the same work while it waits, is one operation; and
// reading an operation's state never moves it.
func TestRemoteBackupPollingNeverStartsSecondUpload(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	req := remoteOperationRequest{Kind: "upload", DestinationID: f.serverDest, Site: f.site(t), BackupName: f.agent.name,
		RequestedBy: int64(f.adminID), RequestIdentity: "0123456789abcdef0123456789abcdef"}
	first, created, _ := f.panel.queueRemoteOperation(ctx, req)
	second, created2, _ := f.panel.queueRemoteOperation(ctx, req)
	req.RequestIdentity = "fedcba9876543210fedcba9876543210"
	third, created3, _ := f.panel.queueRemoteOperation(ctx, req)
	if !created || created2 || created3 || first != second || first != third {
		t.Fatalf("operations %s %s %s (%v %v %v)", first, second, third, created, created2, created3)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/domains/x/backups/remote/operation?id="+first, nil)
	r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: f.adminID, Role: roleAdmin}))
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		f.panel.handleDomainRemoteBackup(w, r, f.domainID, "backup-remote-operation")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"queued"`) {
			t.Fatalf("poll: %d %s", w.Code, w.Body.String())
		}
	}
	var n int
	f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_backup_operations`).Scan(&n)
	if n != 1 || f.store.puts != 0 {
		t.Fatalf("operations %d puts %d", n, f.store.puts)
	}
}

// Rights (D-042): an own destination needs the permission; a server-wide
// destination is chosen by the administrator or when offered; a previous
// owner's destination stops holding a site after a transfer.
func TestRemoteBackupRightsGates(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	customer := &Caller{ID: f.customerID, Role: roleCustomer}
	admin := &Caller{ID: f.adminID, Role: roleAdmin}
	// A Custom subscription without the permission.
	res, err := db.Exec(`INSERT INTO rights_sets (owner_kind, owner_id, all_permissions, all_software) VALUES ('subscription', ?, 0, 1)`, f.subID)
	if err != nil {
		t.Fatal(err)
	}
	setID, _ := res.LastInsertId()
	db.Exec(`INSERT INTO rights_set_items (set_id, item) VALUES (?, 'permission:backups')`, setID)
	if refusal, err := f.panel.ownDestinationRefusal(ctx, customer); err != nil || refusal != rightsLayerSubscription {
		t.Fatalf("without the right: %q %v", refusal, err)
	}
	own, _ := f.panel.loadRemoteDestination(ctx, f.ownDest)
	if code, _ := f.panel.destinationChoiceRefusal(ctx, customer, own, f.site(t)); code != errCodeRightNotGranted {
		t.Fatalf("own destination without the right: %q", code)
	}
	db.Exec(`INSERT INTO rights_set_items (set_id, item) VALUES (?, 'permission:remote_backup_destinations')`, setID)
	if refusal, _ := f.panel.ownDestinationRefusal(ctx, customer); refusal != "" {
		t.Fatalf("with the right: %q", refusal)
	}
	if code, _ := f.panel.destinationChoiceRefusal(ctx, customer, own, f.site(t)); code != "" {
		t.Fatalf("own destination with the right: %q", code)
	}
	// Server-wide, not offered: the administrator only.
	db.Exec(`UPDATE remote_backup_destinations SET offered_to_accounts = 0 WHERE id = ?`, f.serverDest)
	server, _ := f.panel.loadRemoteDestination(ctx, f.serverDest)
	if code, _ := f.panel.destinationChoiceRefusal(ctx, customer, server, f.site(t)); code != "DESTINATION_NOT_AVAILABLE" {
		t.Fatalf("not offered: %q", code)
	}
	if code, _ := f.panel.destinationChoiceRefusal(ctx, admin, server, f.site(t)); code != "" {
		t.Fatalf("administrator: %q", code)
	}
	// Another customer's destination is never usable for this site.
	stranger, _ := db.Exec(`INSERT INTO users (username,password_hash,email,role) VALUES ('rb-stranger','x','s@example.test','customer')`)
	strangerID, _ := stranger.LastInsertId()
	other := f.insertDestination(t, strangerID, false)
	otherDest, _ := f.panel.loadRemoteDestination(ctx, other)
	if destinationUsableForSite(otherDest, f.site(t), true) {
		t.Fatal("a stranger's destination holds this site")
	}
	// After a transfer the previous owner's destination stops at run time.
	newOwner, _ := db.Exec(`INSERT INTO users (username,password_hash,email,role) VALUES ('rb-new','x','n@example.test','customer')`)
	newID, _ := newOwner.LastInsertId()
	db.Exec(`UPDATE subscriptions SET owner_id = ? WHERE id = ?`, newID, f.subID)
	id, _, _ := f.panel.queueRemoteOperation(ctx, remoteOperationRequest{Kind: "upload", DestinationID: f.ownDest,
		Site: f.site(t), BackupName: f.agent.name, RequestedBy: int64(f.customerID)})
	f.panel.runRemoteBackupQueue(ctx)
	op, _ := f.panel.loadRemoteOperation(ctx, id)
	if op.State != remoteOpFailed || op.FailureCode != "DESTINATION_NOT_AVAILABLE" || f.store.puts != 0 {
		t.Fatalf("after transfer: %+v puts %d", op, f.store.puts)
	}
	// Additional users never manage destinations.
	if refusal, _ := f.panel.ownDestinationRefusal(ctx, &Caller{ID: 99, Role: "additional_user", AccountType: "additional_user", CustomerID: f.customerID}); refusal != "role" {
		t.Fatalf("additional user: %q", refusal)
	}
}

// A schedule's copies follow its retention; copies sent by hand are kept.
func TestRemoteBackupScheduleRetention(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	res, err := db.Exec(`INSERT INTO remote_backup_policies (destination_id, domain_id, frequency, backup_type, keep_daily, keep_weekly, created_by)
		VALUES (?, ?, 'daily', 'full', 2, 0, ?)`, f.serverDest, f.domainID, f.adminID)
	if err != nil {
		t.Fatal(err)
	}
	policyID, _ := res.LastInsertId()
	site := f.site(t)
	old := []string{
		"full-20261029T020000.000000000Z-0000000000000001.cpbak",
		"full-20261030T020000.000000000Z-0000000000000002.cpbak",
		"full-20261031T020000.000000000Z-0000000000000003.cpbak",
	}
	for _, name := range old {
		key, _ := remotebackup.ObjectKey("panel.example", site.Name, site.ID, remotebackup.GroupForPolicy(policyID), name)
		f.store.objects[key] = []byte("x")
		f.store.objects[key+".json"] = []byte("{}")
	}
	manualKey, _ := remotebackup.ObjectKey("panel.example", site.Name, site.ID, remotebackup.GroupManual, old[0])
	f.store.objects[manualKey] = []byte("manual")

	f.panel.queueDueRemotePolicies(ctx, time.Now())
	f.panel.queueDueRemotePolicies(ctx, time.Now()) // still waiting: not queued twice
	var queued int
	db.QueryRow(`SELECT COUNT(*) FROM remote_backup_operations WHERE policy_id = ?`, policyID).Scan(&queued)
	if queued != 1 {
		t.Fatalf("queued %d", queued)
	}
	f.panel.runRemoteBackupQueue(ctx)
	if len(f.agent.creates) != 1 || !strings.HasPrefix(f.agent.creates[0].JobKey, remoteJobKeyPrefix) || f.agent.creates[0].Type != "full" {
		t.Fatalf("local backup for the schedule: %+v", f.agent.creates)
	}
	if len(f.agent.deletes) != 1 || f.agent.deletes[0] != f.agent.name {
		t.Fatalf("the schedule's local copy was not removed after the verified upload: %v", f.agent.deletes)
	}
	var keys []string
	for k := range f.store.objects {
		if strings.HasSuffix(k, ".gpg") && strings.Contains(k, "/schedule-") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	// keep_daily 2: the new one (2 Nov) and 31 Oct.
	if len(keys) != 2 || !strings.Contains(keys[0], old[2]) || !strings.Contains(keys[1], f.agent.name) {
		t.Fatalf("after retention: %v", keys)
	}
	if _, ok := f.store.objects[manualKey]; !ok {
		t.Fatal("retention removed a copy sent by hand")
	}
	if _, ok := f.store.objects[strings.Replace(keys[0], old[2], old[0], 1)+".json"]; ok {
		t.Fatal("a removed copy's side file was left")
	}
}

// What a screen receives never carries a secret or the passphrase, and the
// database holds them sealed.
func TestRemoteBackupCredentialsNeverLeave(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	d, _ := f.panel.loadRemoteDestination(ctx, f.serverDest)
	view := remoteDestinationViewFor(&Caller{ID: f.adminID, Role: roleAdmin}, d, 0)
	raw, _ := json.Marshal(view)
	for _, secret := range []string{f.secretKey, f.passphrase, "AKIDTESTKEY1234"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("view carries %q: %s", secret, raw)
		}
	}
	var sealedSecret, sealedPass string
	f.panel.db.GetDB().QueryRow(`SELECT secret_sealed, passphrase_sealed FROM remote_backup_destinations WHERE id = ?`, f.serverDest).Scan(&sealedSecret, &sealedPass)
	if !secrets.IsEncrypted(sealedSecret) || !secrets.IsEncrypted(sealedPass) || strings.Contains(sealedSecret+sealedPass, f.secretKey) {
		t.Fatal("credentials are not sealed at rest")
	}
	// A customer sees an offered server-wide destination without settings.
	cview := remoteDestinationViewFor(&Caller{ID: f.customerID, Role: roleCustomer}, d, int64(f.customerID))
	if cview.Config != nil || cview.Location != "" || cview.Manageable {
		t.Fatalf("customer view of a server-wide destination: %+v", cview)
	}
}

// The connection test confirms an SFTP server's key only after a person
// gives its fingerprint, and refuses a changed key.
func TestRemoteBackupSFTPHostKeyConfirmation(t *testing.T) {
	f := newRemoteFixture(t)
	ctx := context.Background()
	presented := remotebackup.SFTPHostKey{Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl", Fingerprint: "SHA256:presented"}
	oldProbe := probeRemoteHostKey
	probeRemoteHostKey = func(context.Context, string, int) (remotebackup.SFTPHostKey, error) { return presented, nil }
	t.Cleanup(func() { probeRemoteHostKey = oldProbe })
	cfg := remoteDestinationConfig{Host: "backup.example", Port: 22, User: "u", Path: "/b"}
	secret := remoteDestinationSecret{Password: "pw-123456"}
	result := f.panel.testDestination(ctx, remoteDestinationKindSFTP, cfg, secret, "", "")
	if result.OK || result.HostKey == nil || result.Guidance.Code != "REMOTE_HOST_KEY_CONFIRM" || f.store.puts != 0 {
		t.Fatalf("unconfirmed: %+v", result)
	}
	result = f.panel.testDestination(ctx, remoteDestinationKindSFTP, cfg, secret, "", "SHA256:presented")
	if !result.OK || result.pinned != presented.Key {
		t.Fatalf("confirmed: %+v", result)
	}
	// A stored key that the server no longer shows: refused, nothing sent.
	puts := f.store.puts
	stored := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	result = f.panel.testDestination(ctx, remoteDestinationKindSFTP, cfg, secret, stored, "")
	if result.OK || result.Guidance.Code != "REMOTE_HOST_KEY" || f.store.puts != puts {
		t.Fatalf("changed key: %+v", result)
	}
}
