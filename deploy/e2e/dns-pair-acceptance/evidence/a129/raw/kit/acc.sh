export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
adm GET /api/v1/domains/4/access --label access > $T/acc.json
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps({k:v for k,v in b.items() if k!="domain"})[:2500])' $T/acc.json
grun main g_pkgs.sh
