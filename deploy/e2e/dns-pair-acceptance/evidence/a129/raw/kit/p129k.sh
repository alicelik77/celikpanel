# lab 132 label of fix/a129-lab2 (40a021576: filter replaced, certificate folder healed): aru and main 131 -> 132 pressed once,
# Arch PostgreSQL restart from Components + TLS, MariaDB Fail2ban ban; then p129i. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
BUMP=132 PREVC=$(cat $B/c131) bash $A/build.sh refs/remotes/origin/fix/a129-lab2 v0.1.0-alpha.132 > $B/out-build132.out 2>&1
grep -E "^commit|^lab bump|EXIT|FAILED|\"sha256\"" $B/out-build132.out
C12=$(sed -n "s/^lab bump commit //p" $B/out-build132.out | head -1); echo $C12 > $B/c132; bash $A/portal.sh $C12 > $B/portal132.out 2>&1; echo "portal132 exit $?"
bash $A/portalF.sh add v0.1.0-alpha.132 | tail -1; bash $A/portalF.sh latest v0.1.0-alpha.132
echo "== aru 131 -> 132 $(date -u +%T)"
export S=ar IPU=192.0.3.222 CELLX=aru
bash $A/upd128.sh > /dev/null 2>&1; grep -E "s 200|schema_version|== press|^postgresql" $LOGS/C2upd-ar.log | tail -5 | cut -c1-240
(export CELL=aru Z=ar.test MIP=192.0.3.222 BIP=192.0.3.222 IP=192.0.3.222; source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
 exec >> $LOGS/C4remote-aru-postgresql.log 2>&1
 echo "== 132: one apply (the panel's own loop would do it within a minute) $(ts)"; adm PUT /api/v1/database-remote/server '{"apply":true}' --label rs-apply | sbrief 300
 grun main g_pgw.sh)
unset S IPU CELLX
CELL=aru Z=ar.test MIP=192.0.3.222 BIP=192.0.3.222 IP=192.0.3.222 bash $A/k4remote.sh postgresql restart checks fail2ban pass last > /dev/null 2>&1
sed -n "/== 132: one apply/,\$p" $LOGS/C4remote-aru-postgresql.log | cut -c1-300
echo "== main 131 -> 132 $(date -u +%T)"
CELL=r9 IP=192.0.3.190 Z=r9.test bash $A/upd.sh main > /dev/null 2>&1; grep -E "s 200|schema_version|== press" $LOGS/C2-r9-main.log | tail -4 | cut -c1-200
(export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190; source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
 exec >> $LOGS/C4ban-r9.log 2>&1
 echo "== 132: one apply, then the filter on disk $(ts)"; adm PUT /api/v1/database-remote/server '{"apply":true}' --label rs-apply | sbrief 300; sleep 5
 grun main g_f2bv.sh 2>&1 | head -6)
bash $A/k4ban.sh > /dev/null 2>&1; sed -n "/== 132: one apply/,\$p" $LOGS/C4ban-r9.log | cut -c1-300
echo "== p129k part done $(date -u +%T)"
bash $A/p129i.sh
echo "== p129k done $(date -u +%T)"
