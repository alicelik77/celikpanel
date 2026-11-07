export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C7-r7-main.log) 2>&1
N=main; DID=4
cnt() { admn $N GET /api/v1/domains/$DID/backups --label bk | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(len([x for x in b.get("backups") or [] if x.get("origin")=="manual"]))'; }
echo "== (rerun with the lab tool's header read fixed) the same request id twice, 0.3 s apart: files backup $(ts)"
echo "manual backups before: $(cnt)"
python3 -I $A/rq.py $L r7 $N POST /api/v1/domains/$DID/backups '{"type":"files"}' $(rid) 2 0.3
echo "manual backups after: $(cnt)"
echo "== PHP version change, same request id twice $(ts)"
python3 -I $A/rq.py $L r7 $N POST /api/v1/domains/$DID/php '{"php_version":"8.4"}' $(rid) 2 0.3
grun $N g_jr.sh -1min now "php" 10
