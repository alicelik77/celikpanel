#!/bin/bash
# run 4 stop evidence (read only): the first server's mail_profile step failed with firewall_sync_failed. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
exec > >(tee /var/tmp/a114r4/stop-evidence.out) 2>&1
date -u
echo "===== a6-a main setup operation"; opnow a6-a main | cut -c1-2500
bash $H/gq.sh a6-a main "grep PRETTY_NAME /etc/os-release; dovecot --version; echo '-- agent/panel lines about firewall'; journalctl -u celikpanel-agent -u celikpanel-panel --no-pager -o short-iso | grep -iE 'firewall|nft|ufw' | grep -v level=debug | cut -c1-500 | tail -40; echo '-- H1 phrases'; for p in 'could not be checked' 'fail-closed'; do echo \"[\$p] \$(journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o cat | grep -ci \"\$p\")\"; done; journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o short-iso | grep -iE 'could not be checked|fail-closed' | head -6 | cut -c1-400; echo '-- webmail'; journalctl -u celikpanel-agent --no-pager -o short-iso | grep -iE 'webmail|roundcube|snappymail' | cut -c1-300 | tail -12; echo '-- nft'; nft list ruleset 2>&1 | head -60; systemctl is-active nftables firewalld ufw 2>&1 | paste -sd' '; systemctl is-active celikpanel-panel celikpanel-agent dovecot postfix nginx pdns | paste -sd' '" 120000
echo "===== a6-a backup (Ubuntu) H1 phrases"
bash $H/gq.sh a6-a backup "for p in 'could not be checked' 'fail-closed'; do echo \"[\$p] \$(journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o cat | grep -ci \"\$p\")\"; done; journalctl -u celikpanel-agent --no-pager -o short-iso | grep -iE 'firewall' | grep -v level=debug | cut -c1-300 | tail -5"
