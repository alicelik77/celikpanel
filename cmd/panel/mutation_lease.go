package main

import (
	"sync"
	"sync/atomic"
)

// mutationLease is the Panel's in-process host mutation lease (formerly a
// bare sync.Mutex) with a generation that moves each time the lease is taken
// and each time it is given back: odd while held, even while free
// (alpha.125). A read that does not hold the lease (the Components scan)
// notes the generation before it reads and stores its reading only if the
// generation did not move, so a reading that raced a change never replaces
// the record that change writes.
//
// mutationLease, Panel'in süreç içi değişiklik kirasıdır; alındığında ve
// bırakıldığında ilerleyen bir kuşak numarası taşır (tutulurken tek, boşken
// çift). Kirayı tutmayan bir okuma, kuşak değişmediyse okumasını saklar.
type mutationLease struct {
	mu         sync.Mutex
	generation atomic.Uint64
}

func (l *mutationLease) Lock() {
	l.mu.Lock()
	l.generation.Add(1)
}

func (l *mutationLease) TryLock() bool {
	if !l.mu.TryLock() {
		return false
	}
	l.generation.Add(1)
	return true
}

func (l *mutationLease) Unlock() {
	l.generation.Add(1)
	l.mu.Unlock()
}

// Generation returns the current generation and whether the lease is held.
func (l *mutationLease) Generation() (uint64, bool) {
	generation := l.generation.Load()
	return generation, generation%2 == 1
}

// UnchangedSince is called by the holder of the lease. It reports whether
// the lease was free at `generation` and has been taken exactly once since:
// by the caller, with no other change in between.
func (l *mutationLease) UnchangedSince(generation uint64) bool {
	return generation%2 == 0 && l.generation.Load() == generation+1
}
