package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// installFakeDatabaseEngine puts the fake engine behind the deletion's Agent
// seams: the identity read and the bound drop answer as the Agent's
// DatabaseIdentityV1 and DropDatabaseOnceV1 do (cmd/agent/
// database_drop_once.go), the user removals and the rights sweep as the
// Agent's do (nothing while a database of the name exists).
func installFakeDatabaseEngine(t *testing.T, p *Panel, e *fakeDatabaseEngine) {
	t.Helper()
	savedIdentity, savedDrop, savedSweep, savedNative := databaseIdentityAgent, databaseDropOnceAgent, databaseDroppedRightsAgent, p.databaseAccessNative
	databaseIdentityAgent = func(_ *Panel, _ context.Context, req transport.DatabaseIdentityRequest, resp *transport.DatabaseIdentityResponse) error {
		resp.Exists, resp.Identity = e.identity(req.Name)
		return nil
	}
	databaseDropOnceAgent = func(_ *Panel, _ context.Context, req transport.DropDatabaseOnceRequest, resp *transport.DropDatabaseOnceResponse) error {
		exists, identity := e.identity(req.Name)
		switch {
		case !exists:
			resp.Absent = true
		case req.Identity == "" && !req.ByName:
			resp.Error = "no identity of the database to delete was given; nothing was changed"
		case req.Identity != "" && identity != req.Identity:
			resp.Replaced = true
		default:
			if e.loseAnswer {
				e.loseAnswer = false
				if e.dropRuns {
					e.dropByName(req.Name)
				}
				return errors.New("read: connection reset mid-call")
			}
			e.dropByName(req.Name)
			resp.Dropped = true
		}
		return nil
	}
	databaseDroppedRightsAgent = func(_ *Panel, _ context.Context, req transport.DroppedDatabaseRightsRequest, resp *transport.DroppedDatabaseRightsResponse) error {
		if _, there := e.databases[req.Database]; there {
			resp.DatabaseExists, resp.Error = true, "the database exists again"
			return nil
		}
		resp.Applied = true
		return nil
	}
	p.databaseAccessNative = fakeEngineUsers{e}
	t.Cleanup(func() {
		databaseIdentityAgent, databaseDropOnceAgent, databaseDroppedRightsAgent = savedIdentity, savedDrop, savedSweep
		p.databaseAccessNative = savedNative
	})
}

// fakeEngineUsers removes users as the Agent does: never while a database of
// the name exists.
type fakeEngineUsers struct{ e *fakeDatabaseEngine }

func (u fakeEngineUsers) create(context.Context, transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	return transport.DatabaseAccessUserResponse{Applied: true}, nil
}

func (u fakeEngineUsers) remove(_ context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	if _, there := u.e.databases[req.Database]; there && req.DatabaseDropped {
		return transport.DatabaseAccessUserResponse{DatabaseExists: true, Error: "the database exists again"}, nil
	}
	if u.e.usersFail {
		return transport.DatabaseAccessUserResponse{Error: "engine said no"}, nil
	}
	u.e.removed = append(u.e.removed, req.Username)
	return transport.DatabaseAccessUserResponse{Applied: true}, nil
}
