# B lane: the instances (versions) a component row lists, as the drawer reads them. usage: b_inst.sh SERVICE. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
admn $N POST /api/v1/managed-services/scan "{}" --label scan --timeout 300 > $T/ms-b.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id")==sys.argv[2]:
        print("keys", sorted(r.keys()))
        print({k:r.get(k) for k in ("instances","versions","packages","unit","package_versions")})' $T/ms-b.json $1
