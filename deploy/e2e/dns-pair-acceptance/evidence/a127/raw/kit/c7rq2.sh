export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C7-r7-main.log) 2>&1
N=main; DID=4
echo "== site7.test after the PHP change refused while nginx was stopped (c8e load) $(ts)"
adm GET /api/v1/domains/$DID/php --label php | sbrief 400
lab ssh beta1-lab labnet -- "curl -sS -m 5 -o /dev/null -w '%{http_code}\n' -H 'Host: site7.test' http://192.0.3.190/"
echo "== PHP version change, same request id twice, nginx running $(ts)"
python3 -I $A/rq.py $L r7 $N POST /api/v1/domains/$DID/php '{"php_version":"8.4"}' $(rid) 2 0.3
grun $N g_jr.sh -1min now "ApplySitePool|php switch|vhost regen" 10
adm GET /api/v1/domains/$DID/php --label php | sbrief 300
lab ssh beta1-lab labnet -- "curl -sS -m 5 -o /dev/null -w '%{http_code}\n' -H 'Host: site7.test' http://192.0.3.190/"
