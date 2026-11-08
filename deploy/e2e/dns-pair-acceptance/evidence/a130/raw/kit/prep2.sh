# alpha.130 round, before the re-verify: (B) a Boston-like PostgreSQL 16 leftover made at 130 the owner's way
# (16 installed through the panel's PGDG lane, then `apt-get remove postgresql-16` by hand, data kept);
# (A) the MariaDB rule made active again (apply + MariaDB restart from Components). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/P2prep-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1; b1 login $CELL backup > /dev/null 2>&1
N=backup
echo "== B before $(ts)"; grun $N g_pgl.sh; pgrow
admn $N GET "/api/v1/repo/lane?service_id=postgresql" --label lane > $T/lane.json; sbrief 300 < $T/lane.json
SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print(p.get("sha256",""))' $T/lane.json)
[ -n "$SHA" ] && admn $N POST /api/v1/repo/lane "{\"service_id\":\"postgresql\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 300
installc $N postgresql postgresql-16
grun $N g_pgl.sh
echo "-- the owner removes PostgreSQL 16 with apt (data kept) $(ts)"
gshn $N 'DEBIAN_FRONTEND=noninteractive apt-get remove -y -q postgresql-16 >/dev/null 2>&1; echo rc=$?'
grun $N g_pgl.sh
echo "== A: MariaDB rule active again $(ts)"
adm PUT /api/v1/database-remote/server '{"apply":true}' --label rs-apply | sbrief 300
sleep 10; admn main POST /api/v1/service/action '{"name":"mariadb","action":"restart"}' --label svc-restart --timeout 300 | sbrief 200
sleep 30; adm PUT /api/v1/database-remote/server '{"apply":true}' --label rs-apply2 | sbrief 200; sleep 20
grun main g_k2.sh skip; grun main g_k2.sh nft
adm GET /api/v1/database-remote/server --label rs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print(" ", e["engine"], e["rules"], e["state"].get("state"), e["state"].get("name_resolve"), [p["Code"] for p in e["state"].get("problems") or []]) for e in b["engines"]]'
echo "== prep2 done $(ts)"
