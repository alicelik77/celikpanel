#!/bin/bash
# run 6, topology A (Debian 13 first server + Ubuntu 24.04 joined by code, both candidate). Lab only.
# Setup confirmation, J1 + sanity on Debian 13 and on Ubuntu 24.04 (scen_j1), E-new-4 a/b on the joined server
# through the API; c only when a+b took under 40 minutes. Stops at the first J1 failure (FAILFAST).
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6/env.sh
source $H/common.sh
CELL=a7-a; MIP=192.0.3.180; BIP=192.0.3.181; Z=fa7.test; S=shopa7.test; JD=custa7.test; JD2=custb7.test; JD3=custc7.test
OW() { python3 -I $H/owner.py $CELL "$@" 2>&1 | grep -E '^PASS|^FAIL|JID' | cut -c1-1800; }
export CELL MIP BIP
exec > >(tee -a $LOGS/ops6.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
dnsz() { for ip in $MIP $BIP; do echo "   @$ip $1: SOA [$(rcode $ip $1)] MX=[$(dig1 $ip $1 MX)]"; done; }
jfin() { bash $H/gq.sh $CELL backup "journalctl -u celikpanel-panel -b all --no-pager -o short-iso --since '$1' | grep -E 'joined-server deletion|domain deletion' | cut -c1-260 | tail -12"; }
ffchk() { grep -q '^FAIL' $1 && { echo "FAILFAST $2: see above"; echo OPS-EXIT; exit 3; }; return 0; }

echo "== Setup confirmation on both servers $(ts)"
for n in main backup; do o=$(opnow $CELL $n | head -1); echo "   $n: ${o:0:200}"; done
bash $H/gq.sh $CELL main "cat /opt/celikpanel/release.commit; grep PRETTY_NAME /etc/os-release; systemctl is-active celikpanel-panel celikpanel-agent pdns postfix dovecot | paste -sd' '"
bash $H/gq.sh $CELL backup "cat /opt/celikpanel/release.commit; grep PRETTY_NAME /etc/os-release; systemctl is-active celikpanel-panel celikpanel-agent postfix dovecot | paste -sd' '; journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o cat | grep -ciE 'firewall_sync_failed|fail-closed'"

if [ "${SKIP_J1:-0}" != 1 ]; then
echo "== site $S with alice@ and bob@ on the first server $(ts)"
bash $H/mksite.sh $CELL main $S $Z $MIP $BIP
echo "== J1 + sanity, Debian 13 (bob@$S, sender alice@$S) $(ts)"
OW main j1 $S mail.$Z bob $S mail.$Z | tee /var/tmp/a114r6/j1-deb.txt
ffchk /var/tmp/a114r6/j1-deb.txt "J1 Debian 13"
echo "== J1 + sanity, Ubuntu 24.04 (carol@$JD on the joined server, sender alice@$S) $(ts)"
infra delegate $JD ns1.$Z $MIP ns2.$Z $BIP > /dev/null
OW backup mailpair $JD mail.boston.$Z $S mail.$Z
OW backup j1 $JD mail.boston.$Z carol $S mail.$Z | tee /var/tmp/a114r6/j1-ubu.txt
ffchk /var/tmp/a114r6/j1-ubu.txt "J1 Ubuntu 24.04"
fi

E4T0=$(date +%s)
echo "== E-new-4(a): delete $JD (domain with mail) on the joined server $(ts)"
dnsz $JD
JID=$(domain_id $JD backup); echo "   $JD id $JID"
since=$(date -u '+%Y-%m-%d %H:%M:%S')
b1 api $CELL backup DELETE /api/v1/domains/$JID --label delete-$JD --limit 6000 --timeout 400 | brief
bash $H/delwatch.sh $CELL backup $JID $JD 180 | tee /var/tmp/a114r6/delwatch-a.out
sleep 5; dnsz $JD; jfin "$since"

echo "== E-new-4(b): first server's panel stopped during a joined-server deletion $(ts)"
infra delegate $JD2 ns1.$Z $MIP ns2.$Z $BIP > /dev/null
OW backup jdcreate $JD2
sleep 10; dnsz $JD2
JID2=$(domain_id $JD2 backup); echo "   $JD2 id $JID2"
bash $H/gq.sh $CELL main "systemctl stop celikpanel-panel; systemctl is-active celikpanel-panel"
since=$(date -u '+%Y-%m-%d %H:%M:%S')
b1 api $CELL backup DELETE /api/v1/domains/$JID2 --label delete-$JD2 --limit 6000 --timeout 400 | brief
bash $H/delwatch.sh $CELL backup $JID2 $JD2 75 | tee /var/tmp/a114r6/delwatch-b1.out
echo "-- the first server's panel started again $(ts)"
bash $H/gq.sh $CELL main "systemctl start celikpanel-panel; sleep 3; systemctl is-active celikpanel-panel"
bash $H/delwatch.sh $CELL backup $JID2 $JD2 480 | tee /var/tmp/a114r6/delwatch-b2.out
sleep 5; dnsz $JD2; jfin "$since"
E4M=$(( ($(date +%s) - E4T0) / 60 )); echo "   E-new-4 a+b took $E4M min"

if [ $E4M -lt 40 ]; then
echo "== E-new-4(c): as (b), and the joined server reboots while the deletion waits $(ts)"
infra delegate $JD3 ns1.$Z $MIP ns2.$Z $BIP > /dev/null
OW backup jdcreate $JD3
sleep 10; dnsz $JD3
JID3=$(domain_id $JD3 backup); echo "   $JD3 id $JID3"
bash $H/gq.sh $CELL main "systemctl stop celikpanel-panel; systemctl is-active celikpanel-panel"
since=$(date -u '+%Y-%m-%d %H:%M:%S')
b1 api $CELL backup DELETE /api/v1/domains/$JID3 --label delete-$JD3 --limit 6000 --timeout 400 | brief
bash $H/delwatch.sh $CELL backup $JID3 $JD3 30 | tee /var/tmp/a114r6/delwatch-c1.out
echo "-- reboot the joined server $(ts)"
lab reboot $CELL backup 900 | tr -d '\n '; echo
sleep 30
b1 login $CELL backup > /dev/null
bash $H/delwatch.sh $CELL backup $JID3 $JD3 45 | tee /var/tmp/a114r6/delwatch-c2.out
echo "-- the first server's panel started again $(ts)"
bash $H/gq.sh $CELL main "systemctl start celikpanel-panel; sleep 3; systemctl is-active celikpanel-panel"
bash $H/delwatch.sh $CELL backup $JID3 $JD3 480 | tee /var/tmp/a114r6/delwatch-c3.out
sleep 5; dnsz $JD3; jfin "$since"
else
echo "== E-new-4(c): NOT COVERED (time: a+b took $E4M min)"
fi
echo "== A ops done $(ts)"; echo OPS-EXIT
