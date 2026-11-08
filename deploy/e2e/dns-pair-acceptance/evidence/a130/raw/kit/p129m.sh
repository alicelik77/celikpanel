# lab 133 label of fix/a129-lab2 (40a021576; the 132 label was built from a stale bundle = 131 code: filter replaced, certificate folder healed): aru and main 132 -> 133 pressed once,
# Arch PostgreSQL restart from Components + TLS, MariaDB Fail2ban ban; then p129i. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
BUMP=133 PREVC=$(cat $B/c132) bash $A/build.sh refs/remotes/origin/fix/a129-lab2 v0.1.0-alpha.133 > $B/out-build133.out 2>&1
grep -E "^commit|^lab bump|EXIT|FAILED|\"sha256\"" $B/out-build133.out
C13=$(sed -n "s/^lab bump commit //p" $B/out-build133.out | head -1); echo $C13 > $B/c133; bash $A/portal.sh $C13 > $B/portal133.out 2>&1; echo "portal133 exit $?"
bash $A/portalF.sh add v0.1.0-alpha.133 | tail -1; bash $A/portalF.sh latest v0.1.0-alpha.133
echo "== aru 132 -> 133 $(date -u +%T)"
export S=ar IPU=192.0.3.222 CELLX=aru
bash $A/upd128.sh > /dev/null 2>&1; grep -E "s 200|schema_version|== press|^postgresql" $LOGS/C2upd-ar.log | tail -5 | cut -c1-240
(export CELL=aru Z=ar.test MIP=192.0.3.222 BIP=192.0.3.222 IP=192.0.3.222; source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
 exec >> $LOGS/C4remote-aru-postgresql.log 2>&1
 echo "== 133: one apply (the panel's own loop would do it within a minute) $(ts)"; adm PUT /api/v1/database-remote/server '{"apply":true}' --label rs-apply | sbrief 300
 grun main g_pgw.sh)
unset S IPU CELLX
CELL=aru Z=ar.test MIP=192.0.3.222 BIP=192.0.3.222 IP=192.0.3.222 bash $A/k4remote.sh postgresql restart checks fail2ban pass last > /dev/null 2>&1
sed -n "/== 133: one apply/,\$p" $LOGS/C4remote-aru-postgresql.log | cut -c1-300
echo "== main 132 -> 133 $(date -u +%T)"
CELL=r9 IP=192.0.3.190 Z=r9.test bash $A/upd.sh main > /dev/null 2>&1; grep -E "s 200|schema_version|== press" $LOGS/C2-r9-main.log | tail -4 | cut -c1-200
(export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190; source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
 exec >> $LOGS/C4ban-r9.log 2>&1
 echo "== 133: one apply, then the filter on disk $(ts)"; adm PUT /api/v1/database-remote/server '{"apply":true}' --label rs-apply | sbrief 300; sleep 5
 grun main g_f2bv.sh 2>&1 | head -6)
bash $A/k4ban.sh > /dev/null 2>&1; sed -n "/== 133: one apply/,\$p" $LOGS/C4ban-r9.log | cut -c1-300
echo "== p129k part done $(date -u +%T)"
bash $A/rmcell.sh aru > $LOGS/rm-aru.out 2>&1
mv $LOGS/C1fresh-u22.log $LOGS/C1fresh-u22-run1.log 2>/dev/null
S=u22 OS=ubuntu2204 IPF=192.0.3.211 VER=v0.1.0-alpha.129 bash $A/freshV.sh > /dev/null 2>&1
mv $LOGS/C8rb-u22u-main.log $LOGS/C8rb-u22u-main-run1.log 2>/dev/null
CELL=u22f Z=u22.test MIP=192.0.3.211 BIP=192.0.3.211 bash $A/k2rb.sh > /dev/null 2>&1
grep -vE "^ *$" $LOGS/C8rb-u22f-main.log | cut -c1-260 | tail -45
echo "== p129k done $(date -u +%T)"
