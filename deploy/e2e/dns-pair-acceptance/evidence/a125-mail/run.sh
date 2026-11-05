#!/bin/bash
# a125 lab driver (archlinux WSL guest): one Debian 13 overlay VM from the ready base image.
set -u
B=/var/tmp/cp-base-images; W=/var/tmp/a125lab; P=2275
S=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125/lab
EV=$S/evidence; mkdir -p $EV
O="-i $B/id_ed25519 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
G() { ssh $O -p $P root@127.0.0.1 "$@"; }
T() { G "cd /root && $1 ./agent.test -test.run TestLabPasswordlessMail -test.count=1 2>&1 | grep -v -E '^(PASS|ok|=== RUN|--- PASS)'"; }
case "${1:-all}" in
push) scp $O -P $P $S/g_probe.sh root@127.0.0.1:/root/ ;;
boot)
  $B/boot-overlay.sh $W 3072 $P
  scp $O -P $P $S/g_setup.sh $S/g_probe.sh $S/agent.test root@127.0.0.1:/root/ ;;
setup) G bash /root/g_setup.sh 2>&1 | tee $EV/0-setup.txt ;;
before)
  G bash /root/g_probe.sh before 2>&1 | tee $EV/1-before.txt
  G postconf -n > $EV/2-maincf-before.txt
  T CELIKPANEL_LAB_PASSWORDLESS=report > $EV/3-report-before.json; head -c 3000 $EV/3-report-before.json ;;
close)
  T CELIKPANEL_LAB_PASSWORDLESS=close 2>&1 | tee $EV/4-close.txt
  G "postconf -n; echo '--- record'; cat /var/lib/celikpanel-agent-private/mail-passwordless.json" > $EV/5-maincf-after.txt ;;
after)
  G bash /root/g_probe.sh after 2>&1 | tee $EV/6-after.txt
  echo "=== panel account mail through Agent.SendAccountMailV1 (root sendmail)" | tee $EV/7-account-mail.txt
  T "CELIKPANEL_LAB_PASSWORDLESS=send CELIKPANEL_LAB_TO=account@outside.test" 2>&1 | tee -a $EV/7-account-mail.txt
  G "sleep 3; postqueue -f; sleep 2; grep -l 'CelikPanel account mail (lab)' /root/sink/* | wc -l" | sed 's/^/sink files with the account mail: /' | tee -a $EV/7-account-mail.txt
  T CELIKPANEL_LAB_PASSWORDLESS=report > $EV/8-report-after.json; head -c 3000 $EV/8-report-after.json
  G "journalctl -q -o short-iso -t postfix/sendmail -t postfix/postdrop -t postfix/smtpd | grep -E 'not allowed to submit|Relay access denied'" > $EV/9-journal-refusals.txt; cat $EV/9-journal-refusals.txt ;;
again)
  echo "=== second start: nothing to change" | tee $EV/10-idempotent-and-owner.txt
  T CELIKPANEL_LAB_PASSWORDLESS=close 2>&1 | tee -a $EV/10-idempotent-and-owner.txt
  echo "=== the owner opens the sendmail command again (authorized_submit_users = static:anyone)" | tee -a $EV/10-idempotent-and-owner.txt
  G "postconf -e 'authorized_submit_users=static:anyone'"
  T CELIKPANEL_LAB_PASSWORDLESS=close 2>&1 | tee -a $EV/10-idempotent-and-owner.txt
  G "postconf authorized_submit_users" | tee -a $EV/10-idempotent-and-owner.txt
  G "postconf -e 'authorized_submit_users=root, vmail'" ;;
stop) $B/stop-overlay.sh $W; rm -rf $W ;;
esac
