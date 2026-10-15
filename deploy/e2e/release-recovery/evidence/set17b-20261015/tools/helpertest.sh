#!/bin/bash
# set17b: offline test of guest_set17b_restart_delay.py on the WSL host with a dummy worker unit (no product involved).
set -u
H='/mnt/c/CELIKBROS PROJECTS/celikpanel/deploy/e2e/release-recovery/guest_set17b_restart_delay.py'
RID=0123456789abcdef0123456789abcdef
[ -e /root/celikpanel-release-recovery-lab ] && { echo "refusing: private dir exists"; exit 2; }
mkdir -m 700 /root/celikpanel-release-recovery-lab
systemd-run --unit=celikpanel-self-update-$RID.service --property=Type=exec bash -c 'sleep 3; echo CELIKPANEL_UPDATE_CHECKPOINT database_verified_before_start; for i in 1 2 3 4 5 6 7 8 9 10 11 12; do echo tick $(date +%s); sleep 1; done' 
timeout 60 python3 -I "$H" --operation-id $RID --delay-seconds 6 --limit-seconds 40; echo "helper rc=$?"
journalctl -u celikpanel-self-update-$RID.service -o short-unix --no-pager | tail -n 16
cat /root/celikpanel-release-recovery-lab/set17b-restart-delay-$RID.jsonl | cut -c1-260
systemctl stop celikpanel-self-update-$RID.service 2>/dev/null; systemctl reset-failed celikpanel-self-update-$RID.service 2>/dev/null
rm -rf /root/celikpanel-release-recovery-lab
ls -d /root/celikpanel-release-recovery-lab 2>&1
