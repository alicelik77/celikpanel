# alpha.128 lab round (2026-11-07): lab copy of the published alpha.127 tag, the alpha.128 candidate (release/alpha128 head,
# lab 128 label), a lab 129 label of the candidate (rollback case target); lab portals; labnet. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/env.sh
mkdir -p $B
bash $A/sync.sh
echo "start $(ts)"
( bash $A/build.sh refs/tags/v0.1.0-alpha.127 v0.1.0-alpha.127 > $B/out-build127.out 2>&1
  BUMP=128 PREVC=$C127 bash $A/build.sh refs/heads/fix/a128-lab v0.1.0-alpha.128 > $B/out-build128.out 2>&1 ) &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks git >/dev/null 2>&1; echo labnet-tools-ok"
wait $BP; echo "build done $(ts)"
for v in 127 128; do grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"version\"|\"sha256\"" $B/out-build$v.out; done
bash $A/portal.sh $C127 > $B/portal127.out 2>&1; echo "portal127 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal127.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build128.out | head -1); echo $C > $B/c128
bash $A/portal.sh $C > $B/portal128.out 2>&1; echo "portal128 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal128.out
cat $B/portal-v0.1.0-alpha.128/get.sh.diff
PREVV=v0.1.0-alpha.127 NEXTV=v0.1.0-alpha.128 bash $A/portalF.sh up v0.1.0-alpha.127 > $A/portalnet-up.out 2>&1; tail -1 $A/portalnet-up.out
free -m | head -2
echo "phase1 done $(ts)"
