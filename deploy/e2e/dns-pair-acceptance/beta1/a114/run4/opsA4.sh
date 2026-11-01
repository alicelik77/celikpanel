#!/bin/bash
# run 4 (from run 3), topology A (Debian 13 first server + Ubuntu 24.04 joined by code, both candidate). Lab only.
# Sanity E4 Debian + E-new-3 (full autoresponder run on the first server), Sanity E4 Ubuntu, E-new-4 a/b/c on the
# joined server. Stops at the first High failure (FAILFAST): E-new-4 a.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
source $H/common.sh
CELL=a6-a; MIP=192.0.3.180; BIP=192.0.3.181; Z=fa5.test; S=shopa5.test; JD=custa5.test; JD2=custb5.test; JD3=custc5.test
SH="python3 -I $H/shots.py"
OW() { python3 -I $H/owner.py $CELL "$@" 2>&1 | grep -E '^PASS|^FAIL|JID' | cut -c1-1800; }
export CELL MIP BIP
exec > >(tee -a $LOGS/opsA.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
dnsz() { for ip in $MIP $BIP; do echo "   @$ip $1: SOA [$(rcode $ip $1)] MX=[$(dig1 $ip $1 MX)] DKIM=[$(dig1 $ip celik._domainkey.$1 TXT | cut -c1-30)]"; done; }
jfin() { bash $H/gq.sh $CELL backup "journalctl -u celikpanel-panel -b all --no-pager -o short-iso --since '$1' | grep -E 'joined-server deletion|domain deletion' | cut -c1-260 | tail -12"; }

if [ "${SKIP_H1:-0}" != 1 ]; then
echo "== H1: setup finished on both servers (mail step included); Agent/panel journals $(ts)"
h1=0
for n in main backup; do
  o=$(opnow $CELL $n | head -1); echo "   $n: ${o:0:200}"; case "$o" in succeeded*) ;; *) h1=1;; esac
  bash $H/gq.sh $CELL $n "for p in 'could not be checked' 'fail-closed'; do echo \"   [\$p] agent=\$(journalctl -b all -u celikpanel-agent --no-pager -o cat | grep -ci \"\$p\") panel=\$(journalctl -b all -u celikpanel-panel --no-pager -o cat | grep -ci \"\$p\")\"; done; journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o short-iso | grep -iE 'could not be checked|fail-closed' | head -4 | cut -c1-300; echo \"   mail steps:\"; journalctl -b all -u celikpanel-agent --no-pager -o cat | grep -iE 'dovecot.*reload|reload.*dovecot' | tail -3 | cut -c1-240; dovecot --version; systemctl is-active dovecot postfix" > /var/tmp/a114r4/h1-$n.out
  cat /var/tmp/a114r4/h1-$n.out
  grep -E '^\s+\[(could not be checked|fail-closed)\]' /var/tmp/a114r4/h1-$n.out | grep -vq 'agent=0 panel=0' && h1=1
done
[ $h1 = 0 ] || { echo "FAILFAST H1: setup did not finish or the journals name the H1 lines"; exit 2; }
echo "   H1 PASS"
fi
ffchk() { grep -q '^FAIL' $1 && { echo "FAILFAST $2: see above"; exit 3; }; return 0; }

if [ "${SKIP_SANITY:-0}" != 1 ]; then
echo "== site $S with alice@ and bob@ on the first server $(ts)"
bash $H/mksite.sh $CELL main $S $Z $MIP $BIP
echo "== Sanity Debian 13 + E-new-3: autoresponder (2 mails -> 1 reply, no bounce), strip line, Dovecot stopped $(ts)"
OW main autoresp_e4 $S mail.$Z full | tee /var/tmp/a114r4/sanity-deb.txt
ffchk /var/tmp/a114r4/sanity-deb.txt "Sanity Debian 13 / E-new-3"
echo "== Sanity Ubuntu 24.04: customer domain with mail on the joined server, autoresponder $(ts)"
infra delegate $JD ns1.$Z $MIP ns2.$Z $BIP > /dev/null
OW backup mailpair $JD mail.boston.$Z $S mail.$Z
OW backup autoresp_ubuntu $JD mail.boston.$Z $S mail.$Z | tee /var/tmp/a114r4/sanity-ubu.txt
ffchk /var/tmp/a114r4/sanity-ubu.txt "Sanity Ubuntu 24.04"
fi

echo "== E-new-4(a): delete $JD on the joined server from the Domains screen (no reload) $(ts)"
dnsz $JD
JID=$(domain_id $JD backup); echo "   $JD id $JID"
since=$(date -u '+%Y-%m-%d %H:%M:%S')
( bash $H/delwatch.sh $CELL backup $JID $JD 240 > /var/tmp/a114r4/delwatch-a.out 2>&1 ) &
W=$!
sed "s/@JD@/$JD/g" $H/e4a-items.json > /var/tmp/a114r4/e4a-items.json
$SH $CELL backup e4a-domains /var/tmp/a114r4/e4a-items.json --langs en --widths desktop
wait $W; cat /var/tmp/a114r4/delwatch-a.out
sleep 5; dnsz $JD; jfin "$since"
grep -q DELSTATUS-204 /var/tmp/a114r4/delwatch-a.out || { echo "FAILFAST E-new-4(a): deletion-status did not reach 204"; exit 3; }
grep -E '"apiDelete"|"rows naming|pending row' /var/tmp/a114r4/shots/shots.jsonl 2>/dev/null | grep e4a | tail -4 | cut -c1-900

echo "== E-new-4(b): first server's panel stopped during a joined-server deletion $(ts)"
infra delegate $JD2 ns1.$Z $MIP ns2.$Z $BIP > /dev/null
OW backup jdcreate $JD2
sleep 10; dnsz $JD2
JID2=$(domain_id $JD2 backup); echo "   $JD2 id $JID2"
bash $H/gq.sh $CELL main "systemctl stop celikpanel-panel; systemctl is-active celikpanel-panel"
since=$(date -u '+%Y-%m-%d %H:%M:%S')
b1 api $CELL backup DELETE /api/v1/domains/$JID2 --label delete-$JD2 --limit 6000 --timeout 400 | brief
bash $H/delwatch.sh $CELL backup $JID2 $JD2 75
$SH $CELL backup e4b-pending '[{"name":"domains-main-panel-off","path":"/domains","wait":4000}]' --langs en,tr --widths desktop | tail -c 600
echo "-- the first server's panel started again $(ts)"
bash $H/gq.sh $CELL main "systemctl start celikpanel-panel; sleep 3; systemctl is-active celikpanel-panel"
bash $H/delwatch.sh $CELL backup $JID2 $JD2 480
sleep 5; dnsz $JD2; jfin "$since"

echo "== E-new-4(c): as (b), and the joined server reboots while the deletion waits $(ts)"
infra delegate $JD3 ns1.$Z $MIP ns2.$Z $BIP > /dev/null
OW backup jdcreate $JD3
sleep 10; dnsz $JD3
JID3=$(domain_id $JD3 backup); echo "   $JD3 id $JID3"
bash $H/gq.sh $CELL main "systemctl stop celikpanel-panel; systemctl is-active celikpanel-panel"
since=$(date -u '+%Y-%m-%d %H:%M:%S')
b1 api $CELL backup DELETE /api/v1/domains/$JID3 --label delete-$JD3 --limit 6000 --timeout 400 | brief
bash $H/delwatch.sh $CELL backup $JID3 $JD3 30
echo "-- reboot the joined server $(ts)"
lab reboot $CELL backup 900 | tr -d '\n '; echo
sleep 30
b1 login $CELL backup > /dev/null
bash $H/delwatch.sh $CELL backup $JID3 $JD3 45
echo "-- the first server's panel started again $(ts)"
bash $H/gq.sh $CELL main "systemctl start celikpanel-panel; sleep 3; systemctl is-active celikpanel-panel"
bash $H/delwatch.sh $CELL backup $JID3 $JD3 480
sleep 5; dnsz $JD3; jfin "$since"
echo "== A ops done $(ts)"
