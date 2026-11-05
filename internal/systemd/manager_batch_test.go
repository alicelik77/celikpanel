package systemd

import (
	"errors"
	"strings"
	"testing"
)

// One batched `systemctl show -p Id,ExecStart` gives each unit the ExecStart
// lines its own `systemctl show -p ExecStart` printed (alpha.125).
func TestSplitExecStartBlocks(t *testing.T) {
	output := "Id=nginx.service\nExecStart={ path=/usr/sbin/nginx ; argv[]=/usr/sbin/nginx -g daemon on; master_process on; ; ignore_errors=no }\n\n" +
		"Id=pdns.service\nExecStart={ path=/usr/sbin/pdns_server ; argv[]=/usr/sbin/pdns_server --guardian=no --config-dir=/etc/powerdns ; ignore_errors=no }\n\n" +
		"Id=cron.service\n"
	got, ok := splitExecStartBlocks(output, []string{"nginx", "pdns", "cron"})
	if !ok {
		t.Fatal("a well-formed answer was refused")
	}
	if !strings.HasPrefix(got["nginx"], "ExecStart={ path=/usr/sbin/nginx") || !strings.Contains(got["pdns"], "--config-dir") || got["cron"] != "" {
		t.Fatalf("ExecStart lines = %#v", got)
	}
	if _, ok := splitExecStartBlocks("Id=nginx.service\n", []string{"nginx", "pdns"}); ok {
		t.Fatal("an answer with a block missing was accepted")
	}
}

func TestShowExecStartsAsksOnceAndFallsBack(t *testing.T) {
	old := showCommand
	t.Cleanup(func() { showCommand = old })
	calls := 0
	showCommand = func(args ...string) ([]byte, error) {
		calls++
		if strings.Join(args[:3], " ") != "show -p Id,ExecStart" || len(args) != 5 {
			t.Fatalf("argv = %v", args)
		}
		return []byte("Id=a.service\nExecStart={ argv[]=/usr/bin/a -c /etc/a.conf }\n\nId=b.service\n"), nil
	}
	got, ok := showExecStarts([]string{"a", "b"})
	if !ok || calls != 1 || !strings.Contains(got["a"], "-c /etc/a.conf") {
		t.Fatalf("got %#v ok=%v calls=%d", got, ok, calls)
	}
	showCommand = func(...string) ([]byte, error) { return nil, errors.New("bus") }
	if _, ok := showExecStarts([]string{"a"}); ok {
		t.Fatal("a failed batch was used")
	}
}
