# alpha.125 case 3 after the update (pair at 125): timings and answers vs the 124 baseline, compression, an apt install outside the panel. Lab only.
export CELL=p5 IP=192.0.3.190 Z=p5.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C3-$CELL.log) 2>&1
P=$B/perf
echo "== versions $(ts)"; ver main; ver backup
for n in main backup; do python3 -I $A/perf.py $L $CELL $n a125-$n $P; done
echo "== same answers? (volatile fields dropped: scan times, ages, pids, memory)"
for n in main backup; do for r in managed-services scan capabilities setup-components; do
  if diff -q $P/a124-$n-$r.json $P/a125-$n-$r.json >/dev/null; then echo "  $n $r: identical"; else echo "  $n $r: DIFFERENT"; diff $P/a124-$n-$r.json $P/a125-$n-$r.json | head -30 | sed 's/^/     /'; fi
done; done
echo "== compression (a client that accepts gzip) $(ts)"
bash $A/gz.sh $CELL main; bash $A/gz.sh $CELL backup
echo "== a package installed with apt outside the panel (the owner: apt-get install redis-server) $(ts)"
admn main GET /api/v1/managed-services --label ms | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); L=b if isinstance(b,list) else (b.get("services") or []); print("  before:", [ (s.get("id"), s.get("is_installed")) for s in L if s.get("id") in ("redis","memcached")])'
gshn main 'DEBIAN_FRONTEND=noninteractive apt-get install -y -q redis-server >/dev/null 2>&1; echo apt rc=$?; systemctl is-active redis-server'
T0=$(date +%s.%N)
python3 -I $A/scanpoll.py $L $CELL main redis $T0
echo "== post3 done $(ts)"
