#!/bin/bash
# set19: after staging, delete the lab paths (four labs, the run directory, the three dist builds). No qemu may run.
set -u
E=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/wt92/deploy/e2e/release-recovery/evidence/set19-20261016
q=$(pgrep -fc qemu-system || true)
[ "$q" = 0 ] || { echo "refusing: qemu running ($q)"; exit 2; }
{
  echo "cleanup at $(date -u +%FT%TZ); qemu processes: $q"
  for p in /var/tmp/cp-release-drill-s19-catchup /var/tmp/cp-release-drill-s19-catchup-b /var/tmp/cp-release-drill-s19-catchup-c /var/tmp/cp-release-drill-s19-failed            /var/tmp/cp-pair-accept/dist/1743ac69edb0e5723dc60d992b53094dcc4b1431-acceptance-license            /var/tmp/cp-pair-accept/dist/16cbfc877bd46608f8ff37e2a37905a21941db29-acceptance-license            /var/tmp/cp-pair-accept/dist/967f5ff24942782289e05fbc9b9b83e0072e2b90-acceptance-license /var/tmp/cp-set19; do
    if [ -e "$p" ]; then s=$(du -sh "$p" | cut -f1); rm -rf --one-file-system -- "$p"; echo "removed $p ($s) rc=$? exists_after=$([ -e "$p" ] && echo yes || echo no)"; else echo "absent $p"; fi
  done
  df -h /var/tmp | tail -1
} > $E/host/cleanup.txt 2>&1
cat $E/host/cleanup.txt
