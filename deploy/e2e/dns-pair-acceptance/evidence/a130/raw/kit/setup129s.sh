# alpha.130 retest cells: one server at the published 129 through get.sh, one-server setup (web + mail, BIND) through the
# panel API, then the owner's content: admin PHP site with two mailboxes and a MariaDB database, plan Basic with
# customer cust1 and its PHP site; read-only hashes of the native files. env: S OS IPU. Lab only.
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
exec > >(tee -a $LOGS/setup129-$S.log) 2>&1
echo "== provision $(ts) $OS $IP zone=$Z"
b1 provision $CELL main $OS $IP frankfurt ${MEM:-3584} > $LOGS/prov-$CELL-main.log 2>&1; tail -c 200 $LOGS/prov-$CELL-main.log; echo
sethost $CELL main host1.$Z
bash $A/portalF.sh trust $CELL main
echo "== install 129 through get.sh $(ts)"
A114_PORTAL=$B/portal-v0.1.0-alpha.129 timeout 4000 python3 -I $A/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1; echo "install exit $?"
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
echo "== components for the removal retest $(ts)"
admn main GET "/api/v1/repo/lane?service_id=postgresql" --label lane > $T/lane.json
SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print(p.get("sha256",""))' $T/lane.json)
admn main POST /api/v1/repo/lane "{\"service_id\":\"postgresql\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 200
installc main postgresql postgresql-18
gshn main 'mkdir -p /etc/systemd/system/postfix.service.d; printf "[Service]
# owner (lab)
Nice=5
" > /etc/systemd/system/postfix.service.d/owner.conf; printf "# owner notes (lab)
" > /etc/postfix/owner-notes.txt; systemctl daemon-reload; echo owner-files-ok'
gshn main 'dpkg -l | grep -E "^ii" | grep -cE "postgresql|postfix|dovecot"; pg_lsclusters | cat; systemctl is-active postfix dovecot named bind9 2>&1 | tr "
" " "; cat /opt/celikpanel/release.commit 2>/dev/null'
echo "== setup129 $S done $(ts)"
