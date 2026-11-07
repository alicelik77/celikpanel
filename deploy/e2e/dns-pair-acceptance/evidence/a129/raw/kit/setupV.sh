# case 2 (before): one server at the published 127 through get.sh, one-server setup (web + mail, BIND) through the
# panel API, then the owner's content: admin PHP site with two mailboxes and a MariaDB database, plan Basic with
# customer cust1 and its PHP site; read-only hashes of the native files. env: S OS IPU. Lab only.
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
exec > >(tee -a $LOGS/setup127-$S.log) 2>&1
echo "== provision $(ts) $OS $IP zone=$Z"
b1 provision $CELL main $OS $IP frankfurt ${MEM:-3584} > $LOGS/prov-$CELL-main.log 2>&1; tail -c 200 $LOGS/prov-$CELL-main.log; echo
sethost $CELL main host1.$Z
bash $A/portalF.sh trust $CELL main
echo "== install ${VER:-127} through get.sh $(ts)"
A114_PORTAL=$B/portal-${VER:-v0.1.0-alpha.127} timeout 4000 python3 -I $A/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1; echo "install exit $?"
python3 $H/insum.py $LOGS/install-$CELL-main.log main
bash $A/portalF.sh trust $CELL main
if grep -q '"restart_required": true' $LOGS/install-$CELL-main.log; then echo "== the install asked for a restart (kernel upgraded): the owner reboots $(ts)"; lab reboot $CELL main > /dev/null 2>&1; lab wait-ssh $CELL main 300 > /dev/null 2>&1; sleep 20; bash $A/portalF.sh trust $CELL main; b1 sh $CELL main "uname -r; systemctl is-active celikpanel-panel celikpanel-agent | xargs" --label reboot | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'; fi
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), "can_use_panel", d["access"].get("can_use_panel"))'
W=$B/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP frankfurt here
python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]="bind"; json.dump(d,open(p,"w"))' $W/dnshome.json
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names dnshome; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
for i in $(seq 1 20); do
  b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan | cut -c1-600
  echo "== start $(ts) (try $i)"
  b1 start $CELL main --limit 3000 > $LOGS/start-$CELL.json; st start 400 < $LOGS/start-$CELL.json
  grep -q 'server_setup_distribution_updates_unread' $LOGS/start-$CELL.json || break
  echo "   distribution updates not read yet: the owner waits and presses Start again"; sleep 30
done
EVERY=20 TRIES=120 wait_status $CELL main succeeded failed
opnow $CELL main | head -1 | cut -c1-400
echo "== content at 127 $(ts)"
S1=$(mksite main site1.$Z php); echo "site1 id $S1"; echo $S1 > $T/site1.id
for b in box1 box2; do mkcred $b; admn main POST /api/v1/domains/$S1/mail/accounts "{\"address\":\"$b@site1.$Z\",\"password\":\"$(pw $b)\",\"quota_mb\":200}" --label mbox-$b --timeout 300 | sbrief 300; done
mkcred db1; admn main POST /api/v1/domains/$S1/databases "{\"name\":\"shop\",\"type\":\"mysql\",\"password\":\"$(pw db1)\"}" --label db-create --timeout 300 | sbrief 300
admn main POST /api/v1/plans '{"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":4096,"bandwidth_quota_mb":0}' --label plan-create | sbrief 300
PID=$(admn main GET /api/v1/plans --label plans | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("plans") if isinstance(b,dict) else b; print([p["id"] for p in l if p.get("name")=="Basic"][0])'); echo "plan id $PID"; echo $PID > $T/planBasic.id
mkcred cust1; admn main POST /api/v1/users "{\"username\":\"cust1\",\"email\":\"cust1@lab.test\",\"password\":\"$(pw cust1)\",\"role\":\"customer\",\"plan_id\":$PID}" --label user-create | sbrief 300
SUB=$(sub_of cust1); echo "cust1 subscription $SUB"; echo $SUB > $T/sub-cust1.id
infra delegate cust1.$Z ns1.$Z $IP ns2.$Z $IP > /dev/null
as cust1 POST /api/v1/domains/create "{\"domain\":\"cust1.$Z\",\"project_type\":\"php\",\"ssl_type\":\"none\",\"subscription_id\":$SUB}" | sbrief 300
C1=$(domain_id cust1.$Z main); echo "cust1 site id $C1"; echo $C1 > $T/cust1.id
sleep 20
grun main g_snap.sh pre127
gshn main 'cat /etc/os-release | grep PRETTY; php -v 2>/dev/null | head -1; ls /etc/php 2>/dev/null; systemctl is-active postfix dovecot nginx mariadb named bind9 2>&1 | tr "\n" " "; echo; grep -rhE "^\s*PasswordAuthentication" /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null'
for d in site1 cust1; do echo "  $d.$Z: $(lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Host: $d.$Z' http://$IP/" 2>/dev/null)"; done
echo "== setup127 $S done $(ts)"
