package transport

// A database deletion is bound to the database it was asked to delete, not to
// its name (alpha.140, carried item 12). Before the first drop the panel reads
// the database's identity on the engine (DatabaseIdentityV1) and keeps it
// with the deletion; the drop then takes the database only while the engine
// still shows that identity (DropDatabaseOnceV1). A later press of the same
// deletion sends the same identity, so a database of the same name made in
// the meantime - by hand, by a restore, by an installer, by the customer's
// own user - is never dropped by it.
//
// The identity is what the engine itself offers:
//   - PostgreSQL: the database's oid ("postgresql-oid:<oid>"), unique for the
//     life of the cluster's OID counter;
//   - MariaDB on this machine: the database directory's device, inode number,
//     inode generation and birth time ("mariadb-dir:..."); a directory made
//     again has another generation and birth time even when the filesystem
//     gives it the same inode number. Empty when the filesystem does not
//     record a birth time or the directory cannot be read: the identity is
//     then not known.
//
// Silme, silinmesi istenen veritabanının kimliğine bağlıdır, adına değil: aynı
// silmenin ikinci basışı, bu arada aynı adla açılmış bir veritabanını silmez.

// DatabaseIdentityRequest names one database on this machine's engine. Read
// only.
type DatabaseIdentityRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	// Type is "mysql" (MariaDB) or "postgresql".
	Type string `json:"type"`
	Name string `json:"name"`
	// Port names the PostgreSQL cluster of the database's record.
	Port int `json:"port,omitempty"`
}

// DatabaseIdentityResponse: Exists says whether the engine has a database of
// that name now; Identity is its identity, or "" when the engine cannot tell
// one (see above). Refusal: the PostgreSQL cluster cannot answer (nothing was
// read). Error: the read did not complete; nothing was changed.
type DatabaseIdentityResponse struct {
	Exists   bool                    `json:"exists"`
	Identity string                  `json:"identity,omitempty"`
	Refusal  *DatabaseClusterRefusal `json:"refusal,omitempty"`
	Error    string                  `json:"error,omitempty"`
}

// DropDatabaseOnceRequest drops the database of that name only while it is
// the one identified by Identity. ByName (first press only, with an empty
// Identity): the identity could not be known, and the database there now is
// the one the deletion was asked for, so it is dropped by its name, as every
// deletion did before.
type DropDatabaseOnceRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Type                string `json:"type"`
	Name                string `json:"name"`
	Port                int    `json:"port,omitempty"`
	Identity            string `json:"identity,omitempty"`
	ByName              bool   `json:"by_name,omitempty"`
}

// DropDatabaseOnceResponse says what the engine did:
//   - Dropped: the database was dropped now;
//   - Absent: no database of that name exists; nothing was dropped (an
//     earlier press already dropped it, or somebody else did);
//   - Replaced: a database of that name exists, but it is not the identified
//     one (or, without an identity, it cannot be told whether it is): nothing
//     was dropped;
//   - Refusal / Error: nothing was dropped (a refusal of the cluster, or the
//     engine's verified failure of the statement).
type DropDatabaseOnceResponse struct {
	Dropped  bool                    `json:"dropped,omitempty"`
	Absent   bool                    `json:"absent,omitempty"`
	Replaced bool                    `json:"replaced,omitempty"`
	Refusal  *DatabaseClusterRefusal `json:"refusal,omitempty"`
	Error    string                  `json:"error,omitempty"`
}
