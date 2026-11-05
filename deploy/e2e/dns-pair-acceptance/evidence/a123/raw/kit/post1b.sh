source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
source $A/lib11.sh
for c in u22 d12; do CELL=$c; T=$B/t-$c; echo "== $c systemd features"; GQT=60 gq g_sdfeat.sh; done 2>&1 | tee $LOGS/systemd-features-u22-d12.txt
echo "== u22 setup operation (API read, evidence)"; b1 api u22 main GET /api/v1/setup/operation --label ev-op --limit 400000 2>/dev/null | python3 -c '
import json,sys; d=json.load(sys.stdin); j=json.loads(d["body"]); print(d["status"]); print(json.dumps(j, indent=1)[:5000])' > $LOGS/setup-operation-u22.json; head -c 1500 $LOGS/setup-operation-u22.json; echo
export CELL=d12 IP=192.0.3.190 Z=d12.test; T=$B/t-d12
exec > >(tee -a $LOGS/R1pgdg-d12.log) 2>&1
echo "== d12 PGDG 18 preview after setup $(ts)"
lane
echo "-- after the lane (apt simulation, read-only)"; GQT=120 gq g_pgsim.sh
adm GET "/api/v1/service/install-preview?step=postgresql:postgresql-18" --label prev18 --timeout 150 > $T/p18.json; prevsum $T/p18.json
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); json.dump(json.loads(d["body"]), open(sys.argv[2],"w"), indent=1)' $T/p18.json $LOGS/preview-pgdg18-d12.json
echo "-- no install (preview only); installed state after the preview:"; gsh1 'dpkg -l postgresql-18 postgresql-common libpq5 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$2, \$3}"; ls /etc/apt/sources.list.d/'
echo "== done $(ts)"
