# alpha.123 pre-release round (2026-11-05): build the candidate e99fa018e dist (identity alpha.123) + lab portal, bring up labnet. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/env.sh
mkdir -p $B
bash $A/sync.sh
echo "start $(ts)"
( bash $A/build.sh refs/heads/release/alpha123 v0.1.0-alpha.123 > $B/out-build123.out 2>&1 ) &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp >/dev/null 2>&1; echo labnet-tools-ok"
wait $BP; echo "build done $(ts)"; grep -E "^commit|EXIT|budget|\"commit\"|\"version\"|\"sha256\"" $B/out-build123.out
bash $A/portal.sh $C118 > $B/portal123.out 2>&1; echo "portal123 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal123.out; head -20 $B/portal123.out
echo "phase1 done $(ts)"
