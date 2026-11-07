package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129 lab (Debian 12, Ubuntu 22.04; postfwd 1.35): after a reload the
// prefork children kept the old rules, so a limit set after "limits on" was
// not applied (6 of 6 accepted) and a lifted pause answered 550 for minutes.
// postfwd 1.x takes changed rules with a restart; its counters survive in
// --save_rates, in a folder its own user owns.
func TestAptPostfwdTakesChangedRulesWithARestartAndKeepsItsCounters(t *testing.T) {
	apt, _ := outgoingLayoutForFamily("apt")
	arch, _ := outgoingLayoutForFamily("pacman")
	if !apt.applyByRestart || arch.applyByRestart {
		t.Fatalf("restart to apply: apt %v, pacman %v", apt.applyByRestart, arch.applyByRestart)
	}
	dropIn := apt.dropInContent()
	for _, want := range []string{
		"ExecStartPre=+/usr/bin/install -d -o ${RUNAS} -g ${RUNAS} -m 0700 /var/lib/celikpanel-postfwd\n",
		"--keep_rates --save_rates /var/lib/celikpanel-postfwd/rates --daemon",
		"$ARGS --cache=0",
	} {
		if !strings.Contains(dropIn, want) {
			t.Fatalf("apt drop-in lacks %q:\n%s", want, dropIn)
		}
	}
	if strings.Index(dropIn, "ExecStartPre=") > strings.Index(dropIn, "ExecStart=\n") || strings.Contains(arch.dropInContent(), "ExecStartPre") {
		t.Fatalf("drop-in order or Arch line:\n%s\n%s", dropIn, arch.dropInContent())
	}

	host, layout := installFakeOutgoingHost(t)
	layout.applyByRestart = true
	outgoingLayoutFor = func(string) (outgoingLayout, bool) { return layout, true }
	var resp transport.OutgoingMailLimitsApplyResponse
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil || !resp.Applied {
		t.Fatalf("first apply: %+v %v", resp, err)
	}
	host.mu.Lock()
	host.commands = nil
	host.mu.Unlock()
	changed := sampleOutgoingSpec()
	changed.MailboxLimits["a@example.com"] = 3
	changed.Paused = nil // the password change of bad@
	resp = transport.OutgoingMailLimitsApplyResponse{}
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(changed, &resp); err != nil || !resp.Applied || !resp.Verified {
		t.Fatalf("changed apply: %+v %v", resp, err)
	}
	if host.count("systemctl restart postfwd") != 1 || host.count("systemctl reload postfwd") != 0 {
		t.Fatalf("postfwd 1.x must be restarted, not reloaded: %v", host.commands)
	}
	if !strings.Contains(strings.Join(resp.Steps, "|"), "restarted to take the new rules") {
		t.Fatalf("steps: %v", resp.Steps)
	}
	// The changed limit's counter and the resumed mailbox still start again.
	resets := 0
	for _, c := range host.commands {
		if strings.Contains(c, "--delrate sasl_username=a@example.com") || strings.Contains(c, "--delrate sasl_username=bad@example.com") {
			resets++
		}
	}
	if resets != 2 {
		t.Fatalf("resets = %d: %v", resets, host.commands)
	}
}
