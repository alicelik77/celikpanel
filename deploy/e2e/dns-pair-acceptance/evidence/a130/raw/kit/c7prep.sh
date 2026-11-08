# alpha.127 case 7 preparation on the main: a PHP site; a second PHP version (Sury lane, PHP-FPM 8.3) so the version
# can change. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main
exec > >(tee -a $LOGS/C7prep-$CELL-$N.log) 2>&1
DID=$(mksite $N site7.test php); echo "site7.test id $DID"; echo $DID > $T/site7.id
echo "-- Sury lane $(ts)"
admn $N GET "/api/v1/repo/lane?service_id=php-fpm" --label lane > $T/lanep.json; sbrief 600 < $T/lanep.json
SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print(p.get("sha256",""))' $T/lanep.json)
admn $N POST /api/v1/repo/lane "{\"service_id\":\"php-fpm\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 500
echo "-- PHP-FPM 8.3 $(ts)"; installc $N php-fpm php8.3-fpm
admn $N GET /api/v1/managed-services --label ms | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); rows=b if isinstance(b,list) else (b.get("services") or []); [print(" php-fpm versions", r.get("versions")) for r in rows if r.get("id")=="php-fpm"]'
echo "== c7prep done $(ts)"
