# alpha.125 pre-release round (2026-11-05): build the alpha.124 lab copy (published tag) and the alpha.125 candidate dists + lab portals, bring up labnet. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
mkdir -p $B
bash $A/sync.sh
echo "start $(ts)"
( bash $A/build.sh refs/tags/v0.1.0-alpha.124 v0.1.0-alpha.124 > $B/out-build124.out 2>&1; bash $A/build.sh refs/remotes/origin/release/alpha125 v0.1.0-alpha.125 > $B/out-build125.out 2>&1 ) &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks >/dev/null 2>&1; echo labnet-tools-ok"
wait $BP; echo "build done $(ts)"
for v in 124 125; do grep -E "^commit|EXIT|budget|\"commit\"|\"version\"|\"sha256\"" $B/out-build$v.out; done
bash $A/portal.sh $C124 > $B/portal124.out 2>&1; echo "portal124 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal124.out
bash $A/portal.sh $C125 > $B/portal125.out 2>&1; echo "portal125 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal125.out
echo "phase1 done $(ts)"
