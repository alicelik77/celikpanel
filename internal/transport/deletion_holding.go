package transport

// What deletions left, kept 30 days and then removed (alpha.135, owner
// decision of 14 Nov 2026: "30 gün sakla, sonra sil"). Two kinds of item:
//
//   - "mail": the stored mail of a deleted domain or mailbox, moved aside
//     into <mail root>/.celikpanel-quarantine/<name>, where <name> is
//     domain-<domain id>-<16 hex> or mailbox-<account id>-<16 hex>;
//   - "backups": the backups of a deleted domain on this server, left where
//     the backup store keeps them (<backup root>/subscriptions/<S>/domains/<D>);
//     domain ids are never reused (AUTOINCREMENT).
//
// The Panel keeps one record per item; the Agent derives every path from the
// kind and the item's own identifier, never from a path the caller sends.
//
// Silmelerden kalanlar: 30 gün saklanır, sonra silinir.

const (
	HeldItemMail    = "mail"
	HeldItemBackups = "backups"
)

// HeldItemInfo is one item the Agent found in the two holding places.
type HeldItemInfo struct {
	Kind string `json:"kind"`
	// Name: a mail item's folder name; empty for backups.
	Name string `json:"name,omitempty"`
	// SubscriptionID and DomainID: a backups item's scope; for a mail item
	// DomainID is the id its name carries (domain-<id>-...), 0 for a mailbox.
	SubscriptionID int   `json:"subscription_id,omitempty"`
	DomainID       int   `json:"domain_id,omitempty"`
	AccountID      int   `json:"account_id,omitempty"`
	Bytes          int64 `json:"bytes"`
	Files          int   `json:"files,omitempty"`
	// Unknown: the item is there but could not be measured to the end.
	Unknown bool `json:"unknown,omitempty"`
	// Path: where it is, for the administrator to read; never an input.
	Path string `json:"path,omitempty"`
}

// DeletionHoldingInventoryRequest asks for what is in the holding places.
// Read only.
type DeletionHoldingInventoryRequest struct{}

type DeletionHoldingInventoryResponse struct {
	Items []HeldItemInfo `json:"items"`
	// MailError / BackupsError: that place could not be read (the other
	// still answers); an empty list from it is then not "nothing there".
	MailError    string `json:"mail_error,omitempty"`
	BackupsError string `json:"backups_error,omitempty"`
}

// RemoveHeldItemRequest removes exactly one item.
type RemoveHeldItemRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Kind                string `json:"kind"`
	Name                string `json:"name,omitempty"`
	SubscriptionID      int    `json:"subscription_id,omitempty"`
	DomainID            int    `json:"domain_id,omitempty"`
}

// RemoveHeldItemResponse: Removed is true when the item is gone (Absent when
// it was already gone); Error names a verified refusal or failure.
type RemoveHeldItemResponse struct {
	Removed bool   `json:"removed"`
	Absent  bool   `json:"absent,omitempty"`
	Error   string `json:"error,omitempty"`
}
