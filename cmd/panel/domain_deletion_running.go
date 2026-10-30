package main

import "sync"

// A deletion request that is still running in this Panel (final alpha.113
// acceptance run 3, D-024). While it runs, the saved state a status read
// finds is only a moment of it: the DNS step of a paired server may have a
// pending answer from the backup that the same request checks again at
// once, and an Agent job may be running. Until 31 Oct 2026 such a read
// showed that moment as final guidance ("the server administrator must
// inspect ...", or a peer reason's actions) although the request completed
// by itself seconds later. Now a read during the request says the deletion
// is running and is checked again automatically; the reviewed reasons and
// their actions are shown once the request has ended (its own answer, or a
// later read of what it saved).
// Bu Panel'de hâlâ çalışan bir silme isteği: okuma, silmenin sürdüğünü ve
// kendiliğinden yeniden denetlendiğini söyler; kimin ne yapacağı istek
// bittikten sonra gösterilir.

const (
	domainDeletionRunningStage  = "running"
	domainDeletionRunningReason = "deletion_running"
)

const domainDeletionRunningEnglish = "This deletion is running now. When a backup DNS server is paired, its last step waits for the backup to confirm that the zone is gone; that is checked again automatically within the same deletion. Nobody needs to do anything: the result appears here when it ends, usually within a minute or two. Only if the automatic checks end without confirmation does this page say who must act."

// domainDeletionsRunning counts the deletion requests of each domain that
// are running in this process (a domain's deletions are serialized by
// lockDomainSSLOperation; the count keeps a stray overlap honest).
type domainDeletionsRunning struct {
	mu      sync.Mutex
	running map[int]int
}

// begin records a running deletion; the returned function ends it.
func (r *domainDeletionsRunning) begin(domainID int) func() {
	r.mu.Lock()
	if r.running == nil {
		r.running = map[int]int{}
	}
	r.running[domainID]++
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.running[domainID]--; r.running[domainID] <= 0 {
				delete(r.running, domainID)
			}
		})
	}
}

// isRunning is a read; it never starts or changes work.
func (r *domainDeletionsRunning) isRunning(domainID int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[domainID] > 0
}
