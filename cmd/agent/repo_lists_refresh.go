package main

// One bounded refresh of a managed repository's package lists (alpha.121,
// Boston 4 Nov). The install preview of PostgreSQL 18 said "could not be
// checked" with PGDG enabled: the most likely cause was that the repository's
// lists had not been read yet, so apt could not find postgresql-18. The
// preview now refreshes that repository's lists once, exactly as enabling it
// did (apt-get update over its own source file only, no other source, other
// lists kept), and simulates again. It is single-flight per repository, takes
// the package lock without waiting (another package operation: busy), runs at
// most once in repoListsRefreshInterval per repository and is bounded by
// repoListsRefreshBound. It changes only apt's downloaded lists, which apt
// itself refreshes daily; what apt says about the repository goes to its card.
//
// Bir deponun paket listeleri, etkinleştirmedeki gibi yalnız kendi kaynak
// dosyasıyla, bir kez ve sınırlı sürede tazelenir; aynı anda tek tazeleme,
// depo başına aralıklı.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	repoListsRefreshBound    = 45 * time.Second
	repoListsRefreshInterval = 10 * time.Minute
)

var (
	errRepoListsBusy              = errors.New("another package operation is running")
	errRepoListsRecentlyRefreshed = errors.New("the lists were refreshed a moment ago")
	errRepoNotEnabled             = errors.New("the repository is not enabled on this server")
)

var repoListsRefreshState = struct {
	sync.Mutex
	last    map[string]time.Time
	running map[string]bool
}{last: map[string]time.Time{}, running: map[string]bool{}}

// repoListsRefreshNow and repoListsRefreshRun are seams for tests.
var (
	repoListsRefreshNow = time.Now
	repoListsRefreshRun = func(ctx context.Context, aptGet, source string) ([]byte, error) {
		cmd := serviceMutationCommand(ctx, aptGet, "update",
			"-o", "Dir::Etc::sourcelist="+source,
			"-o", "Dir::Etc::sourceparts=/dev/null",
			"-o", "APT::Get::List-Cleanup=0")
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C", "LANG=C")
		return cmd.CombinedOutput()
	}
)

// refreshManagedRepoLists refreshes one enabled managed repository's lists.
func refreshManagedRepoLists(ctx context.Context, aptGet, id string) error {
	if _, ok := enabledRepoLocations()[id]; !ok {
		return errRepoNotEnabled
	}
	state := &repoListsRefreshState
	state.Lock()
	if state.running[id] {
		state.Unlock()
		return errRepoListsBusy
	}
	if last, ok := state.last[id]; ok && repoListsRefreshNow().Sub(last) < repoListsRefreshInterval {
		state.Unlock()
		return errRepoListsRecentlyRefreshed
	}
	state.running[id] = true
	state.Unlock()
	defer func() {
		state.Lock()
		delete(state.running, id)
		state.Unlock()
	}()
	if !packageOperationMu.TryLock() {
		return errRepoListsBusy
	}
	defer packageOperationMu.Unlock()
	state.Lock()
	state.last[id] = repoListsRefreshNow()
	state.Unlock()
	refreshCtx, cancel := context.WithTimeout(ctx, repoListsRefreshBound)
	defer cancel()
	out, err := repoListsRefreshRun(refreshCtx, aptGet, repoSourcePath(id))
	// What apt said about the repository is kept for its card (D-038 §12).
	recordRepoAptErrorsFor(string(out), id)
	if err != nil {
		if refreshCtx.Err() != nil {
			return fmt.Errorf("apt-get update did not finish within %s", repoListsRefreshBound)
		}
		return fmt.Errorf("apt-get update failed: %v", err)
	}
	return nil
}
