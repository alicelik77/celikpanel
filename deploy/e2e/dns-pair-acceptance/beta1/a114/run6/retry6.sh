#!/bin/bash
# run 6: setup failed on both servers on a transient lab DNS failure (package mirror name not resolved). The owner's
# path: plan again and start again on each server (the wizard's try-again), then wait. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6/env.sh
CELL=a7-a
exec > >(tee -a /var/tmp/a114r6/retry6.out) 2>&1
echo "== lab resolver check $(ts)"; lab ssh beta1-lab labnet -- "dig +time=2 +tries=1 +short @192.0.2.53 deb.debian.org A | tail -1; dig +time=2 +tries=1 +short @192.0.2.53 archive.ubuntu.com A | tail -1; dig +time=2 +tries=1 +short @192.0.2.53 security.ubuntu.com A | tail -1"
for n in main backup; do b1 sh $CELL $n "getent hosts deb.debian.org archive.ubuntu.com | head -2" --label resolve --limit 400 | python3 -c 'import json,sys; print("  ", json.load(sys.stdin)["stdout"].strip().replace("\n"," | "))'; done
for n in main backup; do
  echo "== plan + start again on $n $(ts)"
  J=""; [ $n = backup ] && J=""
  b1 plan $CELL $n --limit 60000 > $LOGS/plan-$CELL-$n-retry.json; python3 $H/plansum2.py $LOGS/plan-$CELL-$n-retry.json "plan $n (retry)" | head -5
  b1 start $CELL $n --limit 300 | st start-$n 300
done
TRIES=90 wait_status $CELL main succeeded failed
TRIES=90 wait_status $CELL backup succeeded failed
echo "== retry done $(ts)"
