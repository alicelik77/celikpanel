# run 4 item 1 (A) / item 7 (B): SSH jail tools; Access view, SSH on, admin tools install when missing. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R4ssh-$CELL.log) 2>&1
CU=${CU:-cust118a}; DID=$(cat $T/did); SUB=$(cat $T/${SUBF:-suba})
view() { as $CU GET /api/v1/domains/$DID/access > $T/acc.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); s=b.get("state") or {}; print(d["status"], "server:", json.dumps({k:(s.get("server") or {}).get(k) for k in ("jail_checked","jail_missing","jail_packages","jail_unknown","package_family")}), "account:", json.dumps({k:(s.get("account") or {}).get(k) for k in ("enabled","ssh","chroot_dir","username")}))' $T/acc.json; }
echo "== R4ssh $(ts) $CELL domain $DID"
view
mkcred fa118
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(json.dumps({"enabled":True,"ssh":True,"password":c["password"],"add_keys":[],"remove_keys":[]}))' $T/cred-fa118.json > $T/fa.json
echo "-- file access + SSH on"; as $CU PUT /api/v1/domains/$DID/access @$T/fa.json | sbrief 700
PK=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps(((b.get("state") or {}).get("server") or {}).get("jail_packages") or []))' $T/acc.json)
if [ "$PK" != "[]" ]; then
  echo "-- admin installs the listed tools $PK $(ts)"; AS_T=1; adm POST /api/v1/service/file-access-tools "{\"packages\":$PK}" --label fa-tools --timeout 900 | sbrief 700
  view
  echo "-- file access + SSH on again"; as $CU PUT /api/v1/domains/$DID/access @$T/fa.json | sbrief 700
fi
rm -f $T/fa.json
view
GQT=60 gq g_jail.sh $SUB
echo "== R4ssh done $(ts)"
