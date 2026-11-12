// Package pdnspeerjournal stores the one-time, operation-bound challenge used by
// optional native PowerDNS peer deletion inspection. It is a separate v1 sidecar,
// not a DNS engine receipt or the service-mutation ledger.
//
// The caller must hold and verify the external host and publication locks and
// prove an active running/recovering ledger admission with the exact request,
// owner and ledger attempt. Pending with no ActiveRequestID is not authority.
// That, with the unchanged enrollment, is what the VerifyCurrent callback of
// Publish and ConsumeOnce checks at each write boundary. The native DNS
// evidence of the proof is the caller's to recheck once around each write
// (since alpha.133: before minting, after Publish before the exchange, and
// after ConsumeOnce before terminal success), not at every boundary.
// Publish must finish before any network exchange. Only an authenticated,
// validated native peer response may invoke ConsumeOnce. The caller then
// rechecks current ledger, enrollment and native state before terminal success;
// a difference found there leaves the operation pending, and the next attempt's
// challenge supersedes the consumed record (admissibleSuccessor does not
// compare State).
//
// After a process restart or restored backup, the caller always mints a fresh
// cryptographically random nonce and increasing proof attempt under the active
// admission. Never reuse an outstanding request returned by Read: backup
// restoration can roll this sidecar backwards and this package cannot infer
// that from its own file. A consumed-before-terminal crash needs a fresh proof.
// A fresh challenge of the same operation may carry a higher catalog serial
// only over the identical catalog member set, which the store verifies from
// the members the caller passes (the PowerDNS daemon's own re-stamp); any
// other catalog, enrollment or operation change remains a context mismatch.
// Retire requires exact terminal-ledger proof and is one rename (alpha.133).
// A private stage left by an interrupted write or retirement blocks Read,
// Publish, ConsumeOnce and Retire. Recover, run by the caller under the same
// held authority before any new challenge, finishes or rolls back exactly the
// states such an interruption can leave (recover_linux.go) and refuses every
// other one as Unresolved, which needs a person; it never writes a record.
// Missing or unsafe storage is never interpreted as successful deletion.
//
// Direct privileged owner edits outside the external lock protocol may produce
// an unknown publication. Atomic exchange retains a displaced before-image
// under a private stage name if a changed file is detected; owner inspection is
// then required. The package cannot claim full power-loss durability on a
// filesystem that does not honor file and directory fsync.
package pdnspeerjournal
