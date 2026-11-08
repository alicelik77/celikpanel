# B lane item 6 (Node): install one Node.js version from Components, remove it (the last) -> runtimes/node gone. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B6node-$CELL.log) 2>&1
D=${RT:-/opt/celikpanel/runtimes}
echo "== $(ts) before"; gshn $N "ls -la $D 2>&1 | head"
admn $N GET /api/v1/runtimes/node/lts --label lts | sbrief 300
V=$(admn $N GET /api/v1/runtimes/node/lts --label lts | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print([r["version"] for r in b["releases"] if r["version"].startswith("22.")][0])'); echo "version $V"
[ -n "$V" ] || { echo "no version"; exit 1; }
R=$(rid); admn $N POST /api/v1/runtimes/node "{\"version\":\"$V\",\"request_id\":\"$R\"}" --label node-install --timeout 600 | sbrief 500
svcwaitn $N $R 900
gshn $N "ls -la $D $D/node 2>&1 | head -12"
admn $N GET /api/v1/runtimes/node --label nodes | sbrief 600
IV=$(admn $N GET /api/v1/runtimes/node --label nodes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b if isinstance(b,list) else (b.get("versions") or b.get("runtimes") or b.get("installed") or []); print((l[0].get("version") if l and isinstance(l[0],dict) else (l[0] if l else "")))')
echo "installed version $IV"
admn $N GET "/api/v1/service/removal-impact?service_id=node&version=$IV" --label node-impact | sbrief 600
admn $N DELETE /api/v1/runtimes/node/$IV --label node-rm --timeout 600 | sbrief 500
sleep 3; gshn $N "ls -la $D 2>&1 | head; ls -la $D/node 2>&1 | head -3"
echo "== $(ts) b_node done"
