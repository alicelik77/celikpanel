# Recheck round (2026-11-05): build the candidate 222a48d99 dist (identity alpha.122) + lab portal, bring up labnet, build the escape kit. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/env.sh
mkdir -p $B
bash $A/sync.sh
echo "start $(ts)"
( bash $A/build.sh refs/heads/fix/a123-old-distros v0.1.0-alpha.122 > $B/out-build122.out 2>&1 ) &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp >/dev/null 2>&1; echo labnet-tools-ok"
wait $BP; echo "build done $(ts)"; grep -E "^commit|EXIT|budget|\"commit\"|\"version\"|\"sha256\"" $B/out-build122.out
bash $A/portal.sh $C118 > $B/portal122.out 2>&1; echo "portal122 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal122.out; head -20 $B/portal122.out
echo "== kit build $(ts)"
KT=$B/kit; rm -rf $KT; mkdir -p $KT; git -c safe.directory='*' -C $B/repo-v0.1.0-alpha.122 archive $C118 deploy/e2e/sandbox-escape cmd internal go.mod go.sum | tar -x -C $KT
cd $KT && export GOTOOLCHAIN=local GOFLAGS=-mod=mod GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache && bash deploy/e2e/sandbox-escape/run.sh build > $B/kit-build.out 2>&1; echo "kit build rc=$?"; tail -5 $B/kit-build.out; ls -la $KT/deploy/e2e/sandbox-escape/bin/ 2>&1
echo "phase1 done $(ts)"
