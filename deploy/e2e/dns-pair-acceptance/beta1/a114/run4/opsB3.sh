#!/bin/bash
# B follow-up: the autoresponder again after Dovecot was restarted (the agent's reload had failed), and the
# Dashboard card -> Add domain -> Create domain flow (harness selector fixed). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
CELL=a4-b; IP=192.0.3.152; Z=archa114.test; S=shopb114.test
SH="python3 -I $H/shots.py"; OW="python3 -I $H/owner.py $CELL main"
exec > >(tee -a $LOGS/opsB3.log) 2>&1
echo "== Arch: Dovecot reload refusal evidence (journal) $(ts)"
bash $H/gq.sh $CELL main "journalctl -u dovecot -b -1 --no-pager -o short-iso | grep -E 'SIGHUP|Failed to read configuration' | tail -6; journalctl -u celikpanel-agent -b -1 --no-pager -o short-iso | grep -iE 'autorespond|lmtp' | tail -5"
echo "== autoresponder on Arch after a Dovecot restart (the reboot) $(ts)"
$OW autoresp_e4 $S mail.$Z basic 2>&1 | grep -E '^PASS|^FAIL' | cut -c1-900
bash $H/gq.sh $CELL main "ls -la /var/mail/vhosts/$S/bob/ | grep -i sieve; doveconf -n >/dev/null && echo doveconf-clean; journalctl -u dovecot --no-pager -o cat --since -10min | grep -E 'Failed to read configuration|Not a directory' | tail -3; ls -la /var/spool/postfix/private/dovecot-lmtp"
echo "== Dashboard card -> Add domain (SSL ticked) -> Create domain, no reload $(ts)"
$SH $CELL main b-host2 '[{"name":"dashboard-card","path":"/","wait":3500,"actions":[{"click":"[data-dashboard-no-website=\"archa114.test\"] button"},{"wait":3000},{"clickText":["[role=dialog] button,button","Create domain"]},{"waitGone":["No website for this name yet",240000]},{"wait":5000},{"shot":"after"},{"clickIf":"nav a[href=\"/\"]"},{"wait":4000},{"shot":"dashboard-after"},{"probe":"!!document.querySelector(\"[data-dashboard-no-website]\")","label":"card still shown after SPA navigation"}]}]' --langs en --widths desktop
lab ssh beta1-lab labnet -- "sudo -n tee /root/q-$CELL.sh >/dev/null" < $H/webcheck.sh
lab ssh beta1-lab labnet -- "sudo -n bash /root/q-$CELL.sh $Z" | grep -E '^resolves|^---|code=|issuer|Verify'
echo "== B3 done $(ts)"
