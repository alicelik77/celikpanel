export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/final-$CELL.log) 2>&1
echo "== final state $(ts)"
for n in main backup; do ver $n; gshn $n 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"'; done
admn main GET /api/v1/domains --label domains | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", d.get("id"), d.get("domain_name"), d.get("status"), d.get("project_type")) for d in b]'
admn main GET /api/v1/domains/4/deletion-status --label delst | sbrief 400
grun main g_mig.sh
