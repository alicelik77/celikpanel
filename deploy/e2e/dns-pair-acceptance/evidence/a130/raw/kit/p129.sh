# alpha.129 acceptance round: builds (127, 128 tags; candidate release/alpha129 ea363da4f; lab 130 label), portals, labnet. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
mkdir -p $B $LOGS; bash $A/sync.sh; echo "start $(ts)"
python3 -I $L/lab.py net-up > /dev/null; python3 -I $L/lab.py stop beta1-lab labnet > /dev/null 2>&1
( timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)" ) &
bash $A/build.sh refs/tags/v0.1.0-alpha.127 v0.1.0-alpha.127 > $B/out-build127.out 2>&1
bash $A/build.sh refs/tags/v0.1.0-alpha.128 v0.1.0-alpha.128 > $B/out-build128.out 2>&1
bash $A/build.sh refs/remotes/origin/release/alpha129 v0.1.0-alpha.129 > $B/out-build129.out 2>&1
C9=$(sed -n "s/^commit //p" $B/out-build129.out | head -1); echo $C9 > $B/c129
BUMP=130 PREVC=$C9 bash $A/build.sh refs/remotes/origin/release/alpha129 v0.1.0-alpha.130 > $B/out-build130.out 2>&1
C10=$(sed -n "s/^lab bump commit //p" $B/out-build130.out | head -1); echo $C10 > $B/c130
wait
for v in 127 128 129 130; do grep -E "^commit|^lab bump|EXIT|FAILED|budget|\"sha256\"" $B/out-build$v.out; done
for c in $C127 $C128 $C9 $C10; do bash $A/portal.sh $c > $B/portal-$c.out 2>&1; echo "portal $c exit $?"; done
PREVV=v0.1.0-alpha.128 NEXTV=v0.1.0-alpha.129 bash $A/portalF.sh up v0.1.0-alpha.129 > $B/portalnet-up.out 2>&1; tail -1 $B/portalnet-up.out
bash $A/portalF.sh add v0.1.0-alpha.127 | tail -1; bash $A/portalF.sh add v0.1.0-alpha.130 | tail -1
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks git mariadb-client postgresql-client >/dev/null 2>&1; echo labnet-tools-ok"
free -m | sed -n 2p; echo "phase1 done $(ts)"
