# alpha.127 lab round (2026-11-07): lab copy of the published alpha.126 tag, the alpha.127 candidate (PR #302 head),
# a lab 128 label of the candidate (update target for the rollback case); lab portals; labnet; screenshot tool. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/env.sh
mkdir -p $B
bash $A/sync.sh
echo "start $(ts)"
( bash $A/build.sh refs/tags/v0.1.0-alpha.126 v0.1.0-alpha.126 > $B/out-build126.out 2>&1
  bash $A/build.sh refs/heads/release/alpha127 v0.1.0-alpha.127 > $B/out-build127.out 2>&1
  BUMP=128 bash $A/build.sh refs/heads/release/alpha127 v0.1.0-alpha.128 > $B/out-build128.out 2>&1 ) &
BP=$!
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks >/dev/null 2>&1; echo labnet-tools-ok"
mkdir -p /var/tmp/a114r6/shots-tool; ln -sfn $B/h /var/tmp/a114r6/h
( cd /var/tmp/a114r6/shots-tool && { [ -f package.json ] || echo '{"name":"a114shots","private":true}' > package.json; } && PUPPETEER_SKIP_DOWNLOAD=1 timeout 600 npm install --no-audit --no-fund puppeteer-core@23.11.1 2>&1 | tail -1; node -e "console.log('puppeteer-core', require('puppeteer-core/package.json').version)" )
cp $A/shot.js /var/tmp/a114r6/shot.js
wait $BP; echo "build done $(ts)"
for v in 126 127 128; do grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"version\"|\"sha256\"" $B/out-build$v.out; done
bash $A/portal.sh $C126 > $B/portal126.out 2>&1; echo "portal126 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal126.out
bash $A/portal.sh $C127 > $B/portal127.out 2>&1; echo "portal127 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal127.out
cat $B/portal-v0.1.0-alpha.127/get.sh.diff
C=$(sed -n 's/^lab bump commit //p' $B/out-build128.out | head -1); echo $C > $B/c128
bash $A/portal.sh $C > $B/portal128.out 2>&1; echo "portal128 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal128.out
PREVV=v0.1.0-alpha.126 NEXTV=v0.1.0-alpha.127 bash $A/portalF.sh up v0.1.0-alpha.126 > $A/portalnet-up.out 2>&1; tail -1 $A/portalnet-up.out
bash $A/portalF.sh add v0.1.0-alpha.128 | tail -3
free -m | head -2
echo "phase1 done $(ts)"
