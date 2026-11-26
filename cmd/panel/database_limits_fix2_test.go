package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
)

// Second review of PR #370 (alpha.149), the second fix round.

// readBackLostLimiter accepts writes and then loses the read-back.
type readBackLostLimiter struct {
	fakeRoleLimiter
	reads int
}

func (l *readBackLostLimiter) ReadUserRoleSettings(user string) (services.RoleSettings, error) {
	l.reads++
	if l.reads > 1 {
		return nil, errors.New("connection reset by peer")
	}
	return l.fakeRoleLimiter.ReadUserRoleSettings(user)
}

// Finding 8: a lost read-back carries the "unknown" reason code, so the
// Problems now line says "not known yet", not that the server refused.
func TestALostReadBackIsNotKnownYet(t *testing.T) {
	f := newRightsFixture(t)
	engine := &readBackLostLimiter{fakeRoleLimiter: fakeRoleLimiter{keys: services.RoleKeys, settings: map[string]string{}}}
	err := f.panel.applyDBRoleLimits(context.Background(), engine, 3, "shop_user", services.RoleLimits{StatementSeconds: 60}, false)
	state, code := dbLimitClassOf(err)
	if state != dbLimitUnknown || code != dbReasonUnknown {
		t.Fatalf("a lost read-back: %q %q (%v)", state, code, err)
	}
}
