# lab 131 label of fix/a129-lab2 (certificate root fix) and the Arch check: aru 129 -> 131 pressed once; apply; restart PostgreSQL from Components; TLS connection. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
BUMP=131 PREVC=$(cat $B/c129) bash $A/build.sh refs/remotes/origin/fix/a129-lab2 v0.1.0-alpha.131 > $B/out-build131.out 2>&1
grep -E "^commit|^lab bump|EXIT|FAILED|\"sha256\"" $B/out-build131.out
C11=$(sed -n "s/^lab bump commit //p" $B/out-build131.out | head -1); echo $C11 > $B/c131; bash $A/portal.sh $C11 > $B/portal131.out 2>&1; echo "portal131 exit $?"
bash $A/portalF.sh add v0.1.0-alpha.131 | tail -1; bash $A/portalF.sh latest v0.1.0-alpha.131
export S=ar IPU=192.0.3.222 CELLX=aru
bash $A/upd128.sh > /dev/null 2>&1; grep -E "s 200|schema_version|failed|== press|differences|^<|^>" $LOGS/C2upd-ar.log | tail -12 | cut -c1-240

CELL=aru Z=ar.test MIP=192.0.3.222 BIP=192.0.3.222 IP=192.0.3.222 bash $A/k4remote.sh postgresql restart checks fail2ban pass last > /dev/null 2>&1; sed -n "/4.2 the administrator/,\$p" $LOGS/C4remote-aru-postgresql.log | tail -40 | cut -c1-400
bash $A/portalF.sh latest v0.1.0-alpha.131
CELL=r9 IP=192.0.3.190 Z=r9.test bash $A/upd.sh main > /dev/null 2>&1; grep -E "s 200|schema_version|== press" $LOGS/C2-r9-main.log | tail -4 | cut -c1-200

bash $A/k4ban.sh > /dev/null 2>&1; cat $LOGS/C4ban-r9.log | cut -c1-300
