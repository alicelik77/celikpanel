# alpha.126 case 5: monitoring on the main (126). Known load: one busy core for 120 s on N vCPUs; ranges; problems.
export CELL=${CELL:-p6} Z=${Z:-p6.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}; export IP=${IP:-$MIP}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
N=${N:-main}
exec > >(tee -a $LOGS/C5-$CELL-$N.log) 2>&1
echo "== versions $(ts)"; ver $N
hist() { admn $N GET "/api/v1/metrics/history?range=$1" --label metrics-$1 > $T/h-$1.json; python3 $A/histsum.py $T/h-$1.json "$2" "$3"; }
echo "== ranges (each at most 360 points) $(ts)"; for r in 1h 6h 24h 7d 30d; do hist $r; done
echo "== known load: one busy core (a shell loop) for 120 s $(ts)"
gshn $N 'nproc; uptime; systemd-run --unit=lab-busy -p RuntimeMaxSec=120 sh -c "while :; do :; done"; date -u +%FT%TZ' 
L0=$(date -u +%FT%TZ); sleep 125; L1=$(date -u +%FT%TZ); echo "  load from $L0 to $L1"
gshn $N 'systemctl is-active lab-busy; uptime'
sleep 75
hist 1h $L0 $L1
echo "== problems now: baseline $(ts)"
admn $N GET /api/v1/metrics/problems --label problems > $T/p0.json; python3 $A/probsum.py $T/p0.json
SVC=${SVC:-dovecot}
echo "== the owner stops $SVC outside the panel; Rescan; problems $(ts)"
gshn $N "systemctl stop $SVC; systemctl is-active $SVC"
admn $N POST /api/v1/managed-services/scan '{}' --label scan --timeout 120 | python3 -c 'import json,sys; d=json.load(sys.stdin); print("  scan", d.get("status"), d.get("elapsed"))'
for i in 1 2 3; do admn $N GET /api/v1/metrics/problems --label problems > $T/p1.json; python3 $A/probsum.py $T/p1.json | grep -q "$SVC" && break; sleep 20; done
python3 $A/probsum.py $T/p1.json
echo "== started again; Rescan; the problem clears $(ts)"
gshn $N "systemctl start $SVC; systemctl is-active $SVC"
admn $N POST /api/v1/managed-services/scan '{}' --label scan --timeout 120 | python3 -c 'import json,sys; d=json.load(sys.stdin); print("  scan", d.get("status"), d.get("elapsed"))'
admn $N GET /api/v1/metrics/problems --label problems > $T/p2.json; python3 $A/probsum.py $T/p2.json
echo "== c5 done $(ts)"
