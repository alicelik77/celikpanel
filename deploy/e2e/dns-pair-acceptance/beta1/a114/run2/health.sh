#!/bin/bash
# What a visitor/owner sees right now (used after each reboot): units, panel
# answers, site https, mail send+read, DNS answers from each server. Lab only.
# usage: health.sh LABEL ; env CELL MIP BIP Z S NODES PANEL
set -u
export APC=${APC:-e8e7ab4e6f3cd7564bfaa612838dfa1c3647a75d}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
source $H/common.sh
LABEL=$1
CELL=${CELL:-r6-a}; MIP=${MIP:-192.0.3.116}; BIP=${BIP:-192.0.3.117}; Z=${Z:-r6f113.test}; S=${S:-r6shop113.test}; NODES=${NODES:-main backup}; PANEL=${PANEL:-frankfurt}
SKIP_PANEL=${SKIP_PANEL:-}
echo "== health [$LABEL] $(ts)"
for n in $NODES; do
  echo "-- $n units/timers"
  gsh $n "systemctl is-active celikpanel-panel celikpanel-agent pdns nginx mariadb postfix dovecot 2>/dev/null | paste -sd' '; systemctl is-enabled celikpanel-panel celikpanel-agent 2>/dev/null | paste -sd' '; systemctl list-units --type=service --state=active --no-legend 'php*-fpm*' 2>/dev/null | cut -c1-60; systemctl list-timers --all --no-pager 2>/dev/null | grep -iE 'certbot' | sed 's/  */ /g'; systemctl is-enabled certbot.timer certbot-renew.timer 2>/dev/null | paste -sd' '; uptime -s; cat /proc/sys/kernel/random/boot_id" health-$LABEL-$n 2000
  if [ -z "$SKIP_PANEL" ] || [ "$n" != main ]; then
    b1 api $CELL $n GET /api/v1/setup/operation --label health-$LABEL-op --limit 200000 2>/dev/null | python3 $H/opsum.py | head -2
  fi
done
echo "-- DNS answers"
for ip in $MIP $BIP; do
  echo "   @$ip SOA $Z: $(rcode $ip $Z) | A $S: $(dig1 $ip $S A) | MX $Z: $(dig1 $ip $Z MX)"
done
echo "-- site"
labq webcheck.sh $S 2>&1 | grep -E '^---|code=|^server|^location|title|software' | head -16
labq phpcheck.sh $S 2>&1 | grep -E '^---|code=|^server|x-powered|^body'
echo "-- mail"
python3 -c 'import json,sys; print(json.dumps({"from": json.load(open(sys.argv[1]))["password"], "to": json.load(open(sys.argv[2]))["password"]}))' /var/tmp/cp-beta1/state/$CELL/mbx-alice.json /var/tmp/cp-beta1/state/$CELL/mbx-bob.json > /var/tmp/a114r2/mpw-$CELL.json
lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/mailtest.py mail.$Z alice@$S bob@$S mail.$Z" < /var/tmp/a114r2/mpw-$CELL.json | python3 -c '
import json,sys
d=json.load(sys.stdin)
print("   smtp 587 send:", d.get("smtp_send"), "| cert", (d.get("smtp_587_cert") or {}).get("san"))
print("   imaps 993 read:", d.get("imap_read"), "| cert", (d.get("imaps_993_cert") or {}).get("san"), "|", d.get("imap_select",""), d.get("imap_error",""))'
rm -f /var/tmp/a114r2/mpw-$CELL.json
echo "   delivered to bob's maildir (files in new+cur): $(gsh main "ls /var/mail/vhosts/$S/bob/new /var/mail/vhosts/$S/bob/cur 2>/dev/null | grep -vc ':$\|^$'" maildir-$LABEL 200)"
