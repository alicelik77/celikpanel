#!/bin/bash
# run 4, the alpha.113 Arch server: H2 (rollback probe 113 -> candidate, then a normal update ~11 s after the verified
# rollback; never a stopped panel), then E-new-2(a) (first autoresponder save on the updated server). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
source $H/common.sh
CELL=a6-r; IP=192.0.3.182; Z=ar113.test; S=shopr5.test
OWN() { python3 -I $H/owner.py "$@" 2>&1 | grep -E '^PASS|^FAIL|SUMMARY-LINE|FINAL-STATUS|PROBE-FINAL|SECOND-FINAL|WATCH|JOURNAL|TRIGGER|H2-RESULT|RELOGIN' | cut -c1-4000; }
ver() { b1 api $1 $2 GET /api/v1/panel/version --label version --limit 4000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", sys.argv[1], b.get("version"), (b.get("commit") or "")[:9], "schema", b.get("schema_version"))' $1/$2; }
exec > >(tee -a $LOGS/opsR.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
echo "== site $S with alice@ and bob@ on the alpha.113 Arch server $(ts)"
bash $H/mksite.sh $CELL main $S $Z $IP $IP
bash $H/portalnet.sh trust $CELL main
ver $CELL main
bash $H/gq.sh $CELL main "pacman -Q dovecot pigeonhole 2>&1; dovecot --version"
bash $H/portalnet.sh latest v0.1.0-alpha.114
echo "== H2: rollback probe (bad panel.env key) 113 -> candidate, then the normal update ~11 s after the verified rollback $(ts)"
OWN $CELL main h2seq 11 | tee /var/tmp/a114r4/h2.txt
ver $CELL main
bash $H/gq.sh $CELL main "cat /opt/celikpanel/release.commit; systemctl is-active celikpanel-panel celikpanel-agent; systemctl --failed --no-legend | cut -c1-100; /usr/libexec/celikpanel/recovery status --json 2>/dev/null | head -c 1500"
if grep -q 'H2-RESULT succeeded' /var/tmp/a114r4/h2.txt; then echo "   H2: the normal update succeeded";
elif grep -q '^PASS .*H2 refused with agent_idle' /var/tmp/a114r4/h2.txt || grep -q '^PASS .*start refused at once' /var/tmp/a114r4/h2.txt; then
  echo "-- H2 refused cleanly; one more normal update after 70 s $(ts)"; sleep 70
  OWN $CELL main update2 succeeded | tee /var/tmp/a114r4/h2b.txt
  grep -q 'PASS main: update ends succeeded' /var/tmp/a114r4/h2b.txt || { echo "FAILFAST H2: the later normal update did not succeed"; touch /var/tmp/a114r4/R-UPDATES-DONE; exit 5; }
else echo "FAILFAST H2: see above"; touch /var/tmp/a114r4/R-UPDATES-DONE; exit 4; fi
grep -q '^FAIL' /var/tmp/a114r4/h2.txt && { echo "FAILFAST H2: a FAIL line above"; touch /var/tmp/a114r4/R-UPDATES-DONE; exit 4; }
ver $CELL main
touch /var/tmp/a114r4/R-UPDATES-DONE
echo "== E-new-2(a): first autoresponder save on the updated Arch server $(ts)"
bash $H/portalnet.sh trust $CELL main > /dev/null
bash $H/gq.sh $CELL main "pacman -Q dovecot pigeonhole 2>&1; systemctl show dovecot -p ActiveEnterTimestamp"
OWN $CELL main autoresp_first $S
T2=$(b1 sh $CELL main "date +%s" --label now --limit 100 | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"].strip())')
echo "-- the next save (and mail: 2 delivered, 1 reply, no bounce) $(ts)"
OWN $CELL main autoresp_e4 $S mail.$Z basic
OWN $CELL main reload_clean $T2
echo "== R done $(ts)"
