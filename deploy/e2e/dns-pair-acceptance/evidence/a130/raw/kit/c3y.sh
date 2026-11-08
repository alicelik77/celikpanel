# alpha.128 case 3a (customer directives): the administrator offers additional nginx directives; cust1 (plan Basic, all
# permissions) tries alias /etc, root, a path argument outside its folder and a plain header; then the offer is withdrawn. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C3x-$CELL.log) 2>&1
C1=$(cat $T/cust1.id); D=cust1.$Z
echo "== 3y offer on $(ts)"; adm PUT /api/v1/nginx/directives-offer '{"offered":true}' --label offer-on | sbrief 200
as cust1 GET /api/v1/domains/$C1/web-settings | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], "directives", json.dumps(b.get("directives")))'
for txt in 'location /x/ { alias /etc/; }' 'root /etc;' 'auth_basic \"x\"; auth_basic_user_file /etc/shadow;' 'location /p/ { proxy_pass http://127.0.0.1:1923; }' 'add_header X-Cust-Directive yes always;'; do
  echo "-- $txt"; as cust1 POST /api/v1/domains/$C1/web-settings/directives "{\"text\":\"$txt\"}" | sbrief 300; done
grun main g_curl.sh $D / X-Cust-Directive
as cust1 POST /api/v1/domains/$C1/web-settings/directives '{"text":""}' | sbrief 80
adm PUT /api/v1/nginx/directives-offer '{"offered":false}' --label offer-off | sbrief 200
echo "== c3y done $(ts)"
