#!/bin/bash
# usage: build.sh BRANCH VERSION  -> acceptance-licence dist of the branch head labelled VERSION. Lab only.
# run 3: a failed bundle budget check stops the run (exit 2); no dist is built from those bytes.
set -uo pipefail
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
BR=$1; V=$2
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6
B=/var/tmp/a114r6
R=$B/repo-$V
mkdir -p $B
rm -rf $R
git init -q $R && git -c safe.directory='*' -C $R fetch -q "$H/src.bundle" "refs/heads/$BR:refs/heads/rel" || exit 1
C=$(git -c safe.directory='*' -C $R rev-parse rel)
echo "commit $C"
TOOLS=$B/tools-$C
rm -rf $TOOLS; mkdir -p $TOOLS
git -c safe.directory='*' -C $R archive $C deploy/e2e/dns-pair-acceptance download-portal | tar -x -C $TOOLS
WB=$B/web-$C
rm -rf $WB; mkdir -p $WB
git -c safe.directory='*' -C $R archive $C web | tar -x -C $WB
cd $WB/web && npm ci --no-audit --no-fund > $B/npmci-$C.log 2>&1 && { npx tsc && npx vite build && node scripts/build-recovery-worker.mjs; } > $B/webbuild-$C.log 2>&1 && { node scripts/check-bundle-budget.mjs > $B/budget-$C.log 2>&1 || { echo "BUNDLE BUDGET CHECK FAILED: run stopped (release blocker)"; cat $B/budget-$C.log; exit 2; }; echo "bundle budget check passed"; tail -5 $B/budget-$C.log; } || { echo "WEB BUILD FAILED"; tail -n 30 $B/webbuild-$C.log $B/npmci-$C.log; exit 1; }
rm -rf $R/web; mkdir -p $R/web/dist; cp -a $WB/web/dist/. $R/web/dist/
rm -rf $WB
export CELIKPANEL_REPO=$R CELIKPANEL_DIST_VERSION=$V
cd /tmp
rm -rf /var/tmp/cp-pair-accept/dist/$C-acceptance-license
timeout 3000 bash "$TOOLS/deploy/e2e/dns-pair-acceptance/scripts/build-dist.sh" --acceptance-license $C > $B/build-$C.log 2>&1
echo "EXIT $?"
tail -8 $B/build-$C.log
ls -la /var/tmp/cp-pair-accept/dist/$C-acceptance-license/
cat /var/tmp/cp-pair-accept/dist/$C-acceptance-license/dist.json 2>/dev/null
