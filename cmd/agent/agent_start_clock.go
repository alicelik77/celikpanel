package main

import (
	"fmt"
	"log"
	"time"
)

// The Agent's start, step by step in its journal (alpha.149, lab D5): one
// line per startup step with how long it took and the time since the start
// began, so that a slow start can be placed next time (the lab measured one
// start of 14 s and could not say where it went). Only the step's name and
// two durations are logged: nothing of the server's state, no secret.
//
// Ajan'ın açılışı adım adım: her adım için bir satır ve süresi.

// agentStartClock times the steps of one start.
type agentStartClock struct {
	began, last time.Time
	now         func() time.Time
	logf        func(format string, args ...any)
}

func newAgentStartClock() *agentStartClock {
	now := time.Now()
	return &agentStartClock{began: now, last: now, now: time.Now, logf: log.Printf}
}

// step logs that a step ended now.
func (c *agentStartClock) step(name string) {
	at := c.now()
	c.logf("%s", agentStartLine(name, at.Sub(c.last), at.Sub(c.began)))
	c.last = at
}

// agentStartLine is one step's line.
func agentStartLine(name string, took, total time.Duration) string {
	return fmt.Sprintf("agent start: %s took %s (%s since the start began)", name, took.Round(time.Millisecond), total.Round(time.Millisecond))
}
