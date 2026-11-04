# run 4 item 3 (second attempt): a fresh subscription whose first measurement already includes 2.5 MB. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R4lim2-$CELL.log) 2>&1
pool() { gsh1 "grep -H file_uploads /etc/php/*/fpm/pool.d/site*.conf; ls -la --time-style=+%T /etc/php/8.4/fpm/pool.d/ | grep site"; }
sd() { as cust118d GET /api/v1/my/subscription | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); u=b["subscriptions"][0]["usage"]; d=u["disk"]; print("  disk", u.get("disk_used_bytes"), "/", u.get("disk_limit_bytes"), {k:d.get(k) for k in ("state","measured_at","uploads_blocked","uploads_applied","uploads_note")})'; }
echo "== R4lim2 $(ts)"
mkcred cust118d
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust118d.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-d | sbrief 200; rm -f $T/u.json
SUBD=$(sub_of cust118d); DID=$(newdomain cust118d $SUBD d118lim.test); echo "subD=$SUBD d118lim.test id=$DID"
python3 $A/mkup.py $T/big.json big.bin gen:2500000; python3 $A/mkup.py $T/php.json up.php php
for f in big php; do as cust118d POST "/api/v1/domains/$DID/files?path=/" @$T/$f.json | sbrief 200; done; rm -f $T/big.json
lc phpup - d118lim.test; sd
echo "-- first measurement"; waitround; sd
echo "== disk limit 1 MB $(ts)"; setq $SUBD '{"disk_quota_mb":1}'
waitround; sd; pool | grep -E "site$((DID)).conf|file_uploads"; lc phpup - d118lim.test
echo "== disk limit lifted $(ts)"; setq $SUBD '{"disk_quota_mb":0}'
waitround; sd; pool | grep -E "file_uploads|site$((DID)).conf"; lc phpup - d118lim.test
echo "== R4lim2 done $(ts)"
