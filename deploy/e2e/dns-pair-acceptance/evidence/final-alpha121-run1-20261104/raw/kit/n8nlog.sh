source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/n8nfail-$CELL.log) 2>&1
NID=$(cat $T/nid)
echo "== n8n install log via API $(ts)"
as cust118b GET "/api/v1/domains/$NID/service-apps/logs?lines=80&install=1" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=d["body"]
try:
  j=json.loads(b); L=j.get("lines") or j.get("log") or j
  print(d["status"]); print("\n".join(L) if isinstance(L,list) else json.dumps(L)[:6000])
except Exception: print(d["status"], b[:6000])'
cat > $T/g.sh <<'G'
journalctl -u celiksvcinst-1 --no-pager 2>/dev/null | tail -40 | cut -c1-300
G
cp $T/g.sh $A/g_tmp2.sh; gq g_tmp2.sh
echo "== remove Drupal (catalog app) $(ts)"
DID=$(domain_id bdrup118.test); OP=$(as cust118b GET /api/v1/domains/$DID/apps | jget installed.0.operation_id); echo "drupal domain $DID op $OP"
AS_TIMEOUT=600 as cust118b DELETE /api/v1/domains/$DID/apps/$OP "{\"confirm\":\"bdrup118.test\",\"remove_database\":true,\"remove_data\":true}" | sbrief 500
as cust118b GET /api/v1/domains/$DID/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  after remove installed:", [(x.get("app_id"),x.get("status")) for x in b.get("installed",[])])'
lc get - bdrup118.test ""
echo "== upstream download check from labnet (same NAT egress) $(ts)"
cat > $A/n403.sh <<'N'
for u in https://releases.wikimedia.org/mediawiki/1.43/mediawiki-1.43.11.tar.gz https://download.phpbb.com/pub/release/3.3/3.3.19/phpBB-3.3.19.tar.bz2; do
  echo "$u curl-UA: $(curl -s -o /dev/null -r 0-99 -w '%{http_code}' $u)  Go-UA: $(curl -s -o /dev/null -r 0-99 -A 'Go-http-client/1.1' -w '%{http_code}' $u)  empty-UA: $(curl -s -o /dev/null -r 0-99 -H 'User-Agent:' -w '%{http_code}' $u)"
done
N
nup n403.sh; nrun n403.sh
