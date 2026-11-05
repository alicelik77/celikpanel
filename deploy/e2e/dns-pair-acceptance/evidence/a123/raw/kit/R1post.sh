# alpha.123 wave 1 after setup: BIND identity, host facts, sandbox probe; d12 also the PGDG 18 install preview (no install). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
source $A/lib11.sh; source $A/probe.sh
exec > >(tee -a $LOGS/R1post-$CELL.log) 2>&1
echo "== $CELL BIND unit identity $(ts)"; GQT=60 gq g_bindid3.sh
echo "== $CELL host facts $(ts)"; GQT=60 gq g_facts.sh
echo "== $CELL sandbox probe $(ts)"; probe
if [ "$CELL" = d12 ]; then
  echo "== PGDG 18 preview $(ts)"
  echo "-- before the lane (apt simulation, read-only)"; GQT=120 gq g_pgsim.sh | head -30
  lane
  echo "-- after the lane (apt simulation, read-only)"; GQT=120 gq g_pgsim.sh
  adm GET "/api/v1/service/install-preview?step=postgresql:postgresql-18" --label prev18 --timeout 150 > $T/p18.json; prevsum $T/p18.json
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); json.dump(json.loads(d["body"]), open(sys.argv[2],"w"), indent=1)' $T/p18.json $LOGS/preview-pgdg18-$CELL.json
  echo "-- no install (preview only); installed state after the preview:"; gsh1 'dpkg -l postgresql-18 postgresql-common libpq5 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$2, \$3}"; ls /etc/apt/sources.list.d/'
fi
echo "== R1post $CELL done $(ts)"
