# Recheck round, Debian 12 (d12): sandbox probe; enable PGDG from Components, PGDG PostgreSQL 18 install PREVIEW only (no install). Lab only.
export CELL=d12 IP=192.0.3.190 Z=d12.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/lib.sh
source $A/lib11.sh; source $A/probe.sh
exec > >(tee -a $LOGS/R12D-$CELL.log) 2>&1
echo "== host facts $(ts)"; GQT=60 gq g_facts.sh
echo "== sandbox probe $(ts)"; probe
echo "== PGDG 18 $(ts)"
echo "-- before the lane (apt simulation, read-only)"; GQT=120 gq g_pgsim.sh | head -30
lane
echo "-- after the lane (apt simulation, read-only)"; GQT=120 gq g_pgsim.sh
adm GET "/api/v1/service/install-preview?step=postgresql:postgresql-18" --label prev18 --timeout 150 > $T/p18.json; prevsum $T/p18.json
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); json.dump(json.loads(d["body"]), open(sys.argv[2],"w"), indent=1)' $T/p18.json $LOGS/preview-pgdg18-$CELL.json
echo "-- no install (preview only); installed state after the preview:"; gsh1 'dpkg -l postgresql-18 postgresql-common libpq5 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$2, \$3}"; ls /etc/apt/sources.list.d/'
echo "== R12Dr done $(ts)"
