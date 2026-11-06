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

// The text is neutral: a standalone server has no backup DNS server, so it
// is never told about one (final acceptance run 4, 31 Oct 2026).
const domainDeletionRunningEnglish = "This deletion is running now and is checked automatically. Nobody needs to do anything: check again in a minute. Only if the automatic checks end without confirmation does this page say who must act."

// domainDeletionsRunning counts the deletion requests of each domain that
// are running in this process (a domain's deletions are serialized by
// lockDomainSSLOperation; the count keeps a stray overlap honest).
type domainDeletionsRunning struct {
	mu      sync.Mutex
	running map[int]int
	// steps: the stage the running request of each domain is at now
	// (mail_tls_cleanup, mail_runtime_cleanup, site_cleanup, dns_cleanup,
	// certificate_cleanup, ledger_delete), so a status read can show the
	// progress window's live step (owner's order, 6 Nov 2026: every
	// operation shows what it is doing while it does it). Memory only: it
	// is a moment of a running request, never saved evidence.
	steps map[int]string
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
				delete(r.steps, domainID)
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

// setStep records the stage a running deletion request of this domain is at
// now. Called only by that request, between its stages; a domain with no
// running request keeps nothing.
// Çalışan silme isteğinin şu anki aşaması; yalnız istek kendisi yazar.
func (r *domainDeletionsRunning) setStep(domainID int, stage string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[domainID] <= 0 {
		return
	}
	if r.steps == nil {
		r.steps = map[int]string{}
	}
	r.steps[domainID] = stage
}

// step is a read of the stage the running request is at ("" when no
// request runs or it has not reached its first stage yet).
func (r *domainDeletionsRunning) step(domainID int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[domainID] <= 0 {
		return ""
	}
	return r.steps[domainID]
}
