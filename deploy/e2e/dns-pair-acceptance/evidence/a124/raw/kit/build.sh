#!/bin/bash
# usage: build.sh REF VERSION -> acceptance-licence dist of REF (from src118.bundle) labelled VERSION. Lab only.
set -uo pipefail
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/env.sh
REF=$1; V=$2
R=$B/repo-$V
rm -rf $R
git init -q $R && git -c safe.directory='*' -C $R fetch -q "$A/../src.bundle" "+$REF:refs/tags/rel" || exit 1
C=$(git -c safe.directory='*' -C $R rev-parse rel^{commit})
echo "commit $C"
if [ -n "${BUMP:-}" ]; then
  # lab only: the release identity bump the release commit makes (sequence policy + get.sh bootstrap lines)
  G="git -c safe.directory=* -C $R"
  $G checkout -q --detach $C
  printf 'format=celikpanel-release-sequence-policy-v1
version=v0.1.0-alpha.118
current=118
previous=117
previous_version=v0.1.0-alpha.117
previous_commit=b22348ff86f806c7132cc067221ce74c6fd09cdc
' > $R/deploy/release-sequence-policy
  sed -i 's/^bootstrap_release_sequence=.*/bootstrap_release_sequence=118/; s/^bootstrap_release_version=.*/bootstrap_release_version=v0.1.0-alpha.118/' $R/download-portal/get.sh
  $G -c user.name=lab -c user.email=lab@beta1-lab.test commit -q -am "LAB ONLY: alpha.118 release identity bump on $C"
  C=$($G rev-parse HEAD); echo "lab bump commit $C"; $G show --stat HEAD | tail -3
fi
TOOLS=$B/tools-$C
rm -rf $TOOLS; mkdir -p $TOOLS
git -c safe.directory='*' -C $R archive $C deploy/e2e/dns-pair-acceptance download-portal | tar -x -C $TOOLS
WB=$B/web-$C
rm -rf $WB; mkdir -p $WB
git -c safe.directory='*' -C $R archive $C web | tar -x -C $WB
cd $WB/web && npm ci --no-audit --no-fund > $B/npmci-$C.log 2>&1 && { npx tsc && npx vite build && node scripts/build-recovery-worker.mjs; } > $B/webbuild-$C.log 2>&1 && { node scripts/check-bundle-budget.mjs > $B/budget-$C.log 2>&1 || { echo "BUNDLE BUDGET CHECK FAILED"; cat $B/budget-$C.log; exit 2; }; echo "bundle budget check passed"; } || { echo "WEB BUILD FAILED"; tail -n 30 $B/webbuild-$C.log $B/npmci-$C.log; exit 1; }
rm -rf $R/web; mkdir -p $R/web/dist; cp -a $WB/web/dist/. $R/web/dist/
rm -rf $WB
export CELIKPANEL_REPO=$R CELIKPANEL_DIST_VERSION=$V
cd /tmp
rm -rf /var/tmp/cp-pair-accept/dist/$C-acceptance-license
timeout 3000 bash "$TOOLS/deploy/e2e/dns-pair-acceptance/scripts/build-dist.sh" --acceptance-license $C > $B/build-$C.log 2>&1
echo "EXIT $?"
tail -4 $B/build-$C.log
cat /var/tmp/cp-pair-accept/dist/$C-acceptance-license/dist.json 2>/dev/null | grep -E '"(sha256|version|commit)"'
