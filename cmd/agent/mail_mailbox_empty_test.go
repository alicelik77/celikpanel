package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestCountDoveadmMessages(t *testing.T) {
	n, ok := countDoveadmMessages([]byte("messages=12\nmessages=3\nmessages=0\n"))
	if !ok || n != 15 {
		t.Fatalf("count = %d %v", n, ok)
	}
	if _, ok := countDoveadmMessages([]byte("messages=x\n")); ok {
		t.Fatal("a broken count was accepted")
	}
}

func TestEmptyMailboxRunsDovecotsOwnExpungeAndRecalc(t *testing.T) {
	saved := emptyMailboxRun
	t.Cleanup(func() { emptyMailboxRun = saved })
	var commands []string
	emptyMailboxRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if args[0] == "-f" {
			return []byte("messages=4\nmessages=1\n"), nil
		}
		return nil, nil
	}
	var resp transport.EmptyMailboxResponse
	if err := (&Agent{}).EmptyMailboxV1(&transport.EmptyMailboxRequest{Email: "a@example.com"}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || resp.Messages != 5 || !resp.QuotaRecalculated {
		t.Fatalf("resp = %+v", resp)
	}
	want := []string{
		"doveadm -f flow mailbox status -u a@example.com messages *",
		"doveadm expunge -u a@example.com mailbox * all",
		"doveadm quota recalc -u a@example.com",
	}
	if strings.Join(commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %v", commands)
	}
	resp = transport.EmptyMailboxResponse{}
	_ = (&Agent{}).EmptyMailboxV1(&transport.EmptyMailboxRequest{Email: "not an address"}, &resp)
	if resp.Applied || resp.Error == "" {
		t.Fatalf("an invalid address was emptied: %+v", resp)
	}
}

func TestEmptyMailboxUnknownToDovecotRemovesNothing(t *testing.T) {
	saved := emptyMailboxRun
	t.Cleanup(func() { emptyMailboxRun = saved })
	calls := 0
	emptyMailboxRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls++
		// doveadm exits 67 (EX_NOUSER) for a user it does not know.
		err := exec.Command("sh", "-c", "exit 67").Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("no exit error: %v", err)
		}
		return nil, err
	}
	var resp transport.EmptyMailboxResponse
	_ = (&Agent{}).EmptyMailboxV1(&transport.EmptyMailboxRequest{Email: "a@example.com"}, &resp)
	if resp.Applied || !resp.Unknown || calls != 1 {
		t.Fatalf("resp = %+v calls %d", resp, calls)
	}
}
