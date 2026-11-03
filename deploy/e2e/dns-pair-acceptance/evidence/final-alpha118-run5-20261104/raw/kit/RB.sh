# topology A line 5 (customer B): remote backup to an SFTP target (labnet user rbk, key auth), pin host key,
# send, object listing, restore, manual gpg decrypt. Lab only; key and passphrase never printed.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/RB-$CELL.log) 2>&1
SUBB=$(cat $T/subb); AID=${RBDID:-$(cat $T/aid 2>/dev/null)}
[ -z "$AID" ] && { AID=$(newdomain cust118b $SUBB rb118.test); echo "rb118.test id=$AID"; }
echo "== RB start $(ts) domain id $AID"
nup nrb.sh; nrun nrb.sh setup; nrun nrb.sh hostfp
NRT=30 nrun nrb.sh key > $T/rbkkey; chmod 600 $T/rbkkey
mkcred rbpass
python3 - $T/rbkkey $T/cred-rbpass.json $T/dest.json <<'P'
import json, sys
key = open(sys.argv[1]).read(); pp = json.load(open(sys.argv[2]))["password"] + "-lab-passphrase"
json.dump({"name": "labnet sftp", "kind": "sftp", "passphrase": pp,
           "config": {"host": "192.0.3.1", "port": 22, "user": "rbk", "path": "/home/rbk/backups"},
           "secret": {"private_key": key}}, open(sys.argv[3], "w"))
open(sys.argv[2] + ".pp", "w").write(pp)
P
chmod 600 $T/dest.json $T/cred-rbpass.json.pp
echo "-- test without fingerprint (expect host key to confirm)"
as cust118b POST /api/v1/remote-backup/destinations/test @$T/dest.json > $T/rbt.json; sbrief 900 < $T/rbt.json
FP=$(python3 -c 'import json,sys,re; t=json.load(open(sys.argv[1]))["body"]; m=re.search(r"SHA256:[A-Za-z0-9+/=]+", t); print(m.group(0) if m else "")' $T/rbt.json)
echo "fingerprint offered: $FP"
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d["host_key_fingerprint"]=sys.argv[2]; json.dump(d,open(sys.argv[1],"w"))' $T/dest.json "$FP"
echo "-- create destination with pinned key"
as cust118b POST /api/v1/remote-backup/destinations @$T/dest.json > $T/rbc.json; sbrief 900 < $T/rbc.json
DST=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("destination") or {}).get("id",""))' $T/rbc.json); echo "destination id $DST"
rm -f $T/dest.json
echo "-- local backup (full)"; as cust118b POST /api/v1/domains/$AID/backups '{"type":"full"}' > $T/bk.json; sbrief 500 < $T/bk.json
BN=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("backup") or {}).get("name",""))' $T/bk.json); echo "backup $BN"
echo "-- send"; as cust118b POST /api/v1/domains/$AID/backups/remote/send "{\"backup_name\":\"$BN\",\"destination_id\":$DST}" > $T/rs.json; sbrief 700 < $T/rs.json
OP=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("operation") or {}).get("id",""))' $T/rs.json)
rbop() { local op=$1 i s; for i in $(seq 1 60); do s=$(as cust118b GET "/api/v1/domains/$AID/backups/remote/operation?id=$op" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); o=b.get("operation",b); print(o.get("state"), o.get("phase"), "verified", o.get("verified"), o.get("bytes_done"), "/", o.get("bytes_total"), json.dumps(o.get("guidance"))[:400])'); case "$s" in queued*|running*) sleep 10;; *) echo "  op $op: $s ($(ts))"; return;; esac; done; echo "  op still $s"; }
[ -n "$OP" ] && rbop $OP
echo "-- objects"; as cust118b GET "/api/v1/domains/$AID/backups/remote/objects?destination_id=$DST" > $T/ro.json; sbrief 900 < $T/ro.json
OK=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); o=b.get("objects") or []; print(o[0]["object_key"] if o else "")' $T/ro.json)
echo "-- on the target"; nrun nrb.sh list
echo "-- manual gpg decrypt of $OK"; cat $T/cred-rbpass.json.pp | timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/nrb.sh decrypt $OK"
echo "-- restore"; as cust118b POST /api/v1/domains/$AID/backups/remote/restore "{\"destination_id\":$DST,\"object_key\":\"$OK\",\"confirm\":true}" > $T/rr.json; sbrief 700 < $T/rr.json
OP=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("operation") or {}).get("id",""))' $T/rr.json)
[ -n "$OP" ] && rbop $OP
echo "== RB done $(ts)"
