#!/bin/bash
# set17b: offline test of guest_set17b_restart_delay.py (completion-pending trigger) on the WSL host with a dummy
# worker unit and a dummy marker directory (no product involved); both removed at the end.
set -u
H='/mnt/c/CELIKBROS PROJECTS/celikpanel/deploy/e2e/release-recovery/guest_set17b_restart_delay.py'
RID=0123456789abcdef0123456789abcdee
for d in /root/celikpanel-release-recovery-lab /var/lib/celikpanel-release-transaction; do [ -e $d ] && { echo "refusing: $d exists"; exit 2; }; done
mkdir -m 700 /root/celikpanel-release-recovery-lab /var/lib/celikpanel-release-transaction
touch /var/lib/celikpanel-release-transaction/active
systemd-run --unit=celikpanel-self-update-$RID.service --property=Type=exec bash -c 'sleep 3; mv /var/lib/celikpanel-release-transaction/active /var/lib/celikpanel-release-transaction/completion.pending; echo moved; for i in 1 2 3 4 5 6; do echo tick $(date +%s.%N); sleep 0.2; done; rm -f /var/lib/celikpanel-release-transaction/completion.pending'
timeout 60 python3 -I "$H" --operation-id $RID --delay-seconds 5 --limit-seconds 40; echo "helper rc=$?"
journalctl -u celikpanel-self-update-$RID.service -o short-unix --no-pager | tail -n 12
cut -c1-330 /root/celikpanel-release-recovery-lab/set17b-restart-delay-$RID.jsonl
systemctl stop celikpanel-self-update-$RID.service 2>/dev/null; systemctl reset-failed celikpanel-self-update-$RID.service 2>/dev/null
rm -rf /root/celikpanel-release-recovery-lab /var/lib/celikpanel-release-transaction
ls -d /root/celikpanel-release-recovery-lab /var/lib/celikpanel-release-transaction 2>&1
