#!/bin/bash
# Run 4 after ops: one reboot of each server of the topology; pair: one
# panel-off check on the main; site delete with status polling; Arch: a new
# site afterwards; journal checks on every node. Lab only.
# usage: env CELL MIP BIP Z S PANEL NODES [PANELOFF=1] [NEWSITE=domain]
set -u
export APC=${APC:-e8e7ab4e6f3cd7564bfaa612838dfa1c3647a75d}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
source $H/common.sh
export CELL MIP BIP Z S PANEL NODES
exec > >(tee -a $LOGS/after-$CELL.log) 2>&1
echo "== reboot $NODES $(ts)"
for n in $NODES; do lab reboot $CELL $n 900 > $LOGS/reboot-$CELL-$n.json 2>&1 & done; wait
for n in $NODES; do python3 -c 'import json,sys; t=open(sys.argv[1]).read(); d=json.loads(t[t.find("{"):]); print(sys.argv[2], "rebooted in", d.get("elapsed_seconds"), "s")' $LOGS/reboot-$CELL-$n.json $n; done
sleep 90
bash $H/health.sh after-reboot
if [ -n "${PANELOFF:-}" ]; then
  echo "== panel off on main (panel+agent stopped and disabled, reboot) $(ts)"
  gsh main "systemctl disable --now celikpanel-panel.service celikpanel-agent.service 2>&1 | tail -2; systemctl is-active celikpanel-panel celikpanel-agent | paste -sd' '" mgmt-off 1000
  lab reboot $CELL main 900 | python3 -c 'import json,sys; d=json.load(sys.stdin); print("main rebooted in", d.get("elapsed_seconds"), "s")'
  sleep 90
  gsh main "ss -ltn | grep -c ':1923 ' | sed 's/^/listeners on 1923: /'; systemctl is-active celikpanel-panel celikpanel-agent pdns nginx postfix dovecot opendkim rspamd | paste -sd' '" panel-off-state 1000
  SKIP_PANEL=1 bash $H/health.sh panel-off
  gsh main "systemctl enable --now celikpanel-agent.service celikpanel-panel.service 2>&1 | tail -2; sleep 8; systemctl is-active celikpanel-agent celikpanel-panel | paste -sd' '" mgmt-on 1000
  sleep 15
fi
echo "== site delete $S $(ts)"
SID=$(cat /var/tmp/a114r2/sid-$CELL-$S)
bash $H/delpoll.sh $SID $S
if [ -n "${NEWSITE:-}" ]; then
  echo "== a new site after the delete: $NEWSITE $(ts)"
  infra delegate $NEWSITE ns1.$Z $MIP ns2.$Z $BIP > /dev/null
  b1 api $CELL main POST /api/v1/domains/create "{\"domain\":\"$NEWSITE\",\"project_type\":\"static\",\"ssl_type\":\"letsencrypt\"}" --label newsite-create --limit 4000 | brief
  NID=$(domain_id $NEWSITE); echo "   new site id $NID"
  b1 api $CELL main POST /api/v1/domains/$NID/ssl/letsencrypt '{"auto_renew":true,"include_mail":false,"provider":"letsencrypt","reissue":false,"redirect_http":true}' --label newsite-ssl --limit 4000 --timeout 480 | brief
  labq webcheck.sh $NEWSITE | grep -E '^---|code=|Verify'
  b1 api $CELL main GET /api/v1/dns/engine --label dns-engine --limit 20000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("   DNS card: state", b.get("state"), "pending", b.get("pending_zone_count"), b.get("pending_publications"))'
fi
if [ -n "${EXTRASITE:-}" ]; then
  echo "== one more site create + delete, no reboot: $EXTRASITE $(ts)"
  infra delegate $EXTRASITE ns1.$Z $MIP ns2.$Z $BIP > /dev/null
  b1 api $CELL main POST /api/v1/domains/create "{\"domain\":\"$EXTRASITE\",\"project_type\":\"static\",\"ssl_type\":\"letsencrypt\"}" --label extra-create --limit 4000 | brief
  XID=$(domain_id $EXTRASITE); echo "   extra site id $XID"
  poll_dig $MIP $EXTRASITE A "$MIP" 60; poll_dig $BIP $EXTRASITE A "$MIP" 120
  b1 api $CELL main POST /api/v1/domains/$XID/ssl/letsencrypt '{"auto_renew":true,"include_mail":false,"provider":"letsencrypt","reissue":false,"redirect_http":true}' --label extra-ssl --limit 4000 --timeout 480 | brief
  sleep 10
  bash $H/delpoll.sh $XID $EXTRASITE
fi
echo "== journals, every boot $(ts)"
for n in $NODES; do q $CELL $n jcheck.sh; done
echo "== after done $(ts)"
