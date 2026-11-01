#!/bin/bash
# alpha.114 run 2, topologies C + D (E7). C: Debian 13 + Ubuntu 24.04 pair on alpha.113; D: alpha.113 first
# server (2083) + fresh candidate joined server (1923). Then the owner's Update through the panel API:
# a refused update (lab: private evidence directory mode) for the Details line, D main and C backup and
# C main to the candidate, schema/port/v3 evidence/DNS/zone CRUD, a 114 -> 113 automatic rollback on C main
# (lab trigger: an unknown key in panel.env makes the candidate's start check fail), the update again,
# then Components mail in place on the updated joined server (the owner's Boston case). Lab only.
set -u
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2
source $H/env.sh
source $H/common.sh
exec > >(tee -a $LOGS/cd.log) 2>&1
OWN() { python3 -I $H/owner.py "$@" 2>&1 | grep -E '^PASS|^FAIL|SUMMARY-LINE' | cut -c1-1500; }
v3() { b1 sh $1 $2 "cd /var/lib/celikpanel-agent-private && stat -c 'dir %a' . && for f in dns-engine-state.json dns-engine-ownership-pdns.json dns-engine-switch-journal.json; do [ -f \$f ] && echo \"\$f \$(sha256sum < \$f | cut -c1-16) \$(grep -o '\"schema\":\"[^\"]*\"' \$f | head -2 | tr '\n' ' ')\"; done; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env" --label v3-$2 --limit 4000 | python3 -c 'import json,sys; print("   "+json.load(sys.stdin)["stdout"].strip().replace(chr(10), chr(10)+"   "))'; }
ver() { b1 api $1 $2 GET /api/v1/panel/version --label version --limit 4000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", sys.argv[1], b.get("version"), (b.get("commit") or "")[:9], "schema", b.get("schema_version"))' $1/$2; }
ov() { b1 api $1 $2 GET /api/v1/dns/overview --label dns-overview --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-300; }
bash $H/sync.sh; bash $H/cpshot.sh
echo "== network lab portal (latest = alpha.113) $(ts)"
bash $H/portalnet.sh up v0.1.0-alpha.113
( export CELL=a4-c MIP=192.0.3.160 BIP=192.0.3.161 Z=c114.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.113 BVER=v0.1.0-alpha.113 MKIND=pair-main BKIND=pair-backup-web MMEM=2560 BMEM=2048 SHOTS=0 MAIN_SITE=none TRUST_NODES="main backup"
  timeout 5400 bash $H/pair2.sh > /var/tmp/a114r2/pairC.out 2>&1 ) &
sleep 45
( export CELL=a4-d MIP=192.0.3.170 BIP=192.0.3.171 Z=d114.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.113 BVER=v0.1.0-alpha.114 MKIND=pair-main-web BKIND=pair-backup-web MMEM=2048 BMEM=2304 SHOTS_M=0 SHOTS_B=1 MAIN_SITE=none TRUST_NODES="main" MNAME=dmain BNAME=djoin
  timeout 5400 bash $H/pair2.sh > /var/tmp/a114r2/pairD.out 2>&1 ) &
wait
echo "== C and D set up $(ts)"
tail -n 10 /var/tmp/a114r2/pairC.out | cut -c1-300
tail -n 10 /var/tmp/a114r2/pairD.out | cut -c1-300
echo "== D mixed pair: ports and DNS from both $(ts)"
for ip in 192.0.3.170 192.0.3.171; do for p in 1923 2083; do timeout 5 bash -c "echo > /dev/tcp/$ip/$p" 2>/dev/null && echo "$ip:$p open" || echo "$ip:$p closed"; done; done
for ip in 192.0.3.170 192.0.3.171; do echo "   @$ip SOA d114.test: $(rcode $ip d114.test) | A dmain=$(dig1 $ip dmain.d114.test A) djoin=$(dig1 $ip djoin.d114.test A)"; done
ov a4-d main; ov a4-d backup
echo "== before the update: versions, schema, v3 evidence $(ts)"
for c in a4-c:main a4-c:backup a4-d:main; do ver ${c%%:*} ${c##*:}; done
echo "-- C main"; v3 a4-c main; echo "-- D main"; v3 a4-d main
bash $H/portalnet.sh latest v0.1.0-alpha.114
echo "== a refused update (lab: D main's private DNS evidence directory set to 0750) -> the full line under Details $(ts)"
b1 sh a4-d main "chmod 0750 /var/lib/celikpanel-agent-private; stat -c '%a' /var/lib/celikpanel-agent-private" --label lab-chmod --limit 200 | python3 -c 'import json,sys; print("   lab mode:", json.load(sys.stdin)["stdout"].strip())'
OWN a4-d main update2 failed
b1 sh a4-d main "chmod 0700 /var/lib/celikpanel-agent-private; stat -c '%a' /var/lib/celikpanel-agent-private; journalctl --no-pager -o cat --since -15min | grep -o 'CELIKPANEL_UPDATE_FAILURE.*' | tail -2" --label lab-chmod-back --limit 4000 | python3 -c 'import json,sys; print("   "+json.load(sys.stdin)["stdout"].strip())'
python3 -I $H/shots.py a4-d main d-update-refused '[{"name":"settings-update","path":"/settings","wait":4000,"actions":[{"eval":"document.querySelectorAll(\"details\").forEach(d=>d.open=true)"},{"clickIf":"[aria-expanded=false][aria-controls*=detail i]"},{"wait":1500},{"probe":"[...document.querySelectorAll(\"details,[data-update-outcome],[role=alert]\")].map(e=>e.innerText.replace(/\\s+/g,\" \").slice(0,900))","label":"update notice text"}]},{"name":"dashboard","path":"/","wait":3500}]' --langs en,tr --widths desktop,phone
echo "== the owner's Update: D main, C backup, C main (attempt 1 rolls back) $(ts)"
OWN a4-d main update2 succeeded
ver a4-d main; v3 a4-d main; ov a4-d main; ov a4-d backup
OWN a4-c backup update2 succeeded
ver a4-c backup
echo "-- C main: lab trigger for an automatic rollback (unknown key in panel.env; the candidate's start check refuses it)"
b1 sh a4-c main "echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; stat -c '%a %U' /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env" --label lab-trigger --limit 300 | python3 -c 'import json,sys; print("   "+json.load(sys.stdin)["stdout"].strip())'
OWN a4-c main update2 recovered
ver a4-c main; v3 a4-c main; ov a4-c main
b1 sh a4-c main "journalctl --no-pager -o cat --since -30min | grep -E 'CELIKPANEL_UPDATE_FAILURE|rollback|rolled back|refus' | grep -v 'level=debug' | cut -c1-300 | tail -15; cat /opt/celikpanel/release.commit 2>/dev/null; /usr/libexec/celikpanel/recovery status --json 2>/dev/null | head -c 1200" --label rollback-journal --limit 20000 | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "-- the alpha.113 panel serves DNS after the rollback"
for ip in 192.0.3.160 192.0.3.161; do echo "   @$ip SOA c114.test: $(rcode $ip c114.test) | MX $(dig1 $ip c114.test MX)"; done
infra delegate zr113.test ns1.c114.test 192.0.3.160 ns2.c114.test 192.0.3.161 > /dev/null
OWN a4-c main zonecrud zr113.test 192.0.3.160 192.0.3.161
python3 -I $H/shots.py a4-c main c-rolled-back '[{"name":"settings-update","path":"/settings","wait":4000,"actions":[{"eval":"document.querySelectorAll(\"details\").forEach(d=>d.open=true)"},{"wait":1000},{"probe":"[...document.querySelectorAll(\"details,[role=alert],[role=status]\")].map(e=>e.innerText.replace(/\\s+/g,\" \").slice(0,700))","label":"update notice text"}]}]' --langs en,tr --widths desktop
echo "-- C main: trigger removed, the update again"
b1 sh a4-c main "sed -i '/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d' /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env" --label lab-trigger-off --limit 300 | python3 -c 'import json,sys; print("   trigger lines left:", json.load(sys.stdin)["stdout"].strip())'
OWN a4-c main update2 succeeded
ver a4-c main; v3 a4-c main
echo "== after the updates: DNS overview and zone CRUD $(ts)"
for c in a4-c:main a4-c:backup a4-d:main a4-d:backup; do echo "-- $c: $(ov ${c%%:*} ${c##*:})"; done
infra delegate zc114.test ns1.c114.test 192.0.3.160 ns2.c114.test 192.0.3.161 > /dev/null
OWN a4-c main zonecrud zc114.test 192.0.3.160 192.0.3.161
infra delegate zd114.test ns1.d114.test 192.0.3.170 ns2.d114.test 192.0.3.171 > /dev/null
OWN a4-d main zonecrud zd114.test 192.0.3.170 192.0.3.171
python3 -I $H/shots.py a4-c main c-after-update '[{"name":"dashboard","path":"/","wait":4000},{"name":"dns-page","path":"/dns-infrastructure","wait":3500},{"name":"settings","path":"/settings","wait":3500}]' --langs en,tr --widths desktop
echo "== the owner's Boston case: Components -> mail in place on the updated joined server C backup $(ts)"
for ip in 192.0.3.160 192.0.3.161; do echo "   before @$ip c114.test MX=[$(dig1 $ip c114.test MX)] SPF=[$(dig1 $ip c114.test TXT | cut -c1-60)] DMARC=[$(dig1 $ip _dmarc.c114.test TXT | cut -c1-50)] DKIM=[$(dig1 $ip celik._domainkey.c114.test TXT | cut -c1-30)]"; done
OWN a4-c backup mailext_taken mail.c114.test
OWN a4-c backup mailext mail.boston.c114.test
for ip in 192.0.3.160 192.0.3.161; do echo "   after  @$ip c114.test MX=[$(dig1 $ip c114.test MX)] SPF=[$(dig1 $ip c114.test TXT | cut -c1-60)] DMARC=[$(dig1 $ip _dmarc.c114.test TXT | cut -c1-50)] DKIM=[$(dig1 $ip celik._domainkey.c114.test TXT | cut -c1-30)] mail.boston A=[$(dig1 $ip mail.boston.c114.test A)]"; done
opnow a4-c backup | head -3
python3 -I $H/shots.py a4-c backup c-backup-after-mail '[{"name":"components","path":"/services","wait":4000},{"name":"dns-page","path":"/dns-infrastructure","wait":3500}]' --langs en,tr --widths desktop
echo "== C/D done $(ts)"
