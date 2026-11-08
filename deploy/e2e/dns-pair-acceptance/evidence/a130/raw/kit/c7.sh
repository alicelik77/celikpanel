# alpha.127 case 7: confirmed changes show the server's steps in the locked window (owner, first login: tour pending),
# EN; PHP and service also TR; a repeated request with the same request id runs once. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main; DID=$(cat $T/site7.id)
exec > >(tee -a $LOGS/C7-$CELL-$N.log) 2>&1
admn $N PUT /api/v1/me/guide-state '{"tour_version":1,"tour_state":"pending"}' --label tour-pending | sbrief 300
admn $N GET /api/v1/me/guide-state --label guide | sbrief 300
python3 - $T <<'P'
import json, subprocess, sys
T = sys.argv[1]; A = "/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit"
def gen(kind, dom, extra=""):
    out = f"{T}/it-{kind}.json"; subprocess.run(["python3", f"{A}/uiitems.py", kind, dom, out, extra], check=True, capture_output=True); return json.load(open(out))
skip = {"eval": "(() => { const b = [...document.querySelectorAll('button,a')].find(e => /^(Skip the tour|Turu atla)/.test(e.innerText.trim())); if (b) b.click(); })()", "after": 800}
rest = gen("op-backup", "site7.test") + gen("op-restore", "site7.test") + gen("op-ssl", "site7.test") + gen("op-service", "mariadb")
rest[0]["actions"].insert(0, skip)
en = gen("op-php", "site7.test") + rest
dns = gen("op-dns", "site7.test", "labrec")
dns[0]["actions"].insert(0, skip)
json.dump(en + dns, open(f"{T}/c7-en.json", "w"))
tr = gen("op-php", "site7.test") + gen("op-service", "mariadb")
json.dump(tr, open(f"{T}/c7-tr.json", "w"))
P
echo "== EN $(ts)"; timeout 3000 python3 -I $H/shots.py $CELL $N c7b $T/c7-en.json --langs en --widths desktop | python3 $A/shotsum.py
echo "== TR $(ts)"; timeout 1500 python3 -I $H/shots.py $CELL $N c7tr $T/c7-tr.json --langs tr --widths desktop | python3 $A/shotsum.py
admn $N GET /api/v1/me/guide-state --label guide | sbrief 300
echo "== a repeated request with the same request id (files backup, two requests 0.3 s apart) $(ts)"
admn $N GET /api/v1/domains/$DID/backups --label bk | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("backups before:", len(b.get("backups") or []))'
python3 -I $A/rq.py $L $CELL $N POST /api/v1/domains/$DID/backups '{"type":"files"}' $(rid) 2 0.3
admn $N GET /api/v1/domains/$DID/backups --label bk | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("backups after:", len(b.get("backups") or []))'
echo "== same for a PHP version change $(ts)"
python3 -I $A/rq.py $L $CELL $N POST /api/v1/domains/$DID/php '{"php_version":"8.3"}' $(rid) 2 0.3
gshn $N "journalctl -u celikpanel-agent --since -2min -o cat --no-pager | grep -ciE 'php.*(pool|version).*site7|site7.*php'"
echo "== c7 done $(ts)"
